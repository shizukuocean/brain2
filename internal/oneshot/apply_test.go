package oneshot

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyInstallsBacksUpWritesReceiptAndRestarts(t *testing.T) {
	root := t.TempDir()
	plan := testPlan(t, root, []testFile{{name: "app.exe", old: "old-app", next: "new-app"}})
	waited := []int{}
	restarted := false
	plan.WaitPIDs = []int{111, 222}
	plan.RestartExecutable = plan.Files[0].TargetPath
	plan.RestartArguments = []string{"--after-update"}

	receipt, err := Apply(context.Background(), plan, ApplyOptions{
		WaitForPID: func(_ context.Context, pid int) error {
			waited = append(waited, pid)
			return nil
		},
		Restart: func(executable string, arguments []string) error {
			restarted = executable == plan.RestartExecutable && len(arguments) == 1 && arguments[0] == "--after-update"
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := readText(t, plan.Files[0].TargetPath); got != "new-app" {
		t.Fatalf("target = %q", got)
	}
	if got := readText(t, filepath.Join(plan.BackupRoot, "0000.bak")); got != "old-app" {
		t.Fatalf("backup = %q", got)
	}
	if fmt.Sprint(waited) != "[111 222]" || !restarted || receipt.Status != "installed" || !receipt.Restarted {
		t.Fatalf("unexpected result: waited=%v restarted=%v receipt=%+v", waited, restarted, receipt)
	}
	var diskReceipt Receipt
	if err := json.Unmarshal([]byte(readText(t, plan.ReceiptPath)), &diskReceipt); err != nil {
		t.Fatal(err)
	}
	if diskReceipt.SchemaVersion != ReceiptSchemaVersion || diskReceipt.TransactionID != plan.TransactionID {
		t.Fatalf("disk receipt = %+v", diskReceipt)
	}
}

func TestApplyRollsBackEveryCommittedFile(t *testing.T) {
	root := t.TempDir()
	plan := testPlan(t, root, []testFile{
		{name: "app.exe", old: "old-app", next: "new-app"},
		{name: "resources/data.bin", old: "old-data", next: "new-data"},
	})
	forced := errors.New("forced second-file failure")
	receipt, err := Apply(context.Background(), plan, ApplyOptions{
		BeforeCommit: func(index int, _ FileEntry) error {
			if index == 1 {
				return forced
			}
			return nil
		},
	})
	if !errors.Is(err, forced) || receipt.Status != "failed" || receipt.ErrorCode != "INSTALL_FAILED" {
		t.Fatalf("Apply() err=%v receipt=%+v", err, receipt)
	}
	if got := readText(t, plan.Files[0].TargetPath); got != "old-app" {
		t.Fatalf("first target was not restored: %q", got)
	}
	if got := readText(t, plan.Files[1].TargetPath); got != "old-data" {
		t.Fatalf("second target changed: %q", got)
	}
}

func TestApplyRollsBackApplicationWhenAfterCommitEntryFails(t *testing.T) {
	root := t.TempDir()
	plan := testPlan(t, root, []testFile{{name: "app.exe", old: "old-app", next: "new-app"}})
	forced := errors.New("shortcut creation failed")
	restarted := false
	plan.RestartExecutable = plan.Files[0].TargetPath
	receipt, err := Apply(context.Background(), plan, ApplyOptions{
		AfterCommit: func() error { return forced },
		Restart: func(string, []string) error {
			restarted = true
			return nil
		},
	})
	if !errors.Is(err, forced) || receipt.Status != "failed" || receipt.ErrorCode != "INSTALL_FAILED" {
		t.Fatalf("Apply() err=%v receipt=%+v", err, receipt)
	}
	if restarted {
		t.Fatal("application restarted after the post-install entry failed")
	}
	if got := readText(t, plan.Files[0].TargetPath); got != "old-app" {
		t.Fatalf("target was not restored after shortcut failure: %q", got)
	}
}

func TestApplyRemovesOnlyDeclaredOldFiles(t *testing.T) {
	root := t.TempDir()
	plan := testPlan(t, root, []testFile{{name: "app.exe", old: "old", next: "new"}})
	stale := filepath.Join(plan.TargetRoot, "stale.dll")
	keep := filepath.Join(plan.TargetRoot, "user-notes.txt")
	writeText(t, stale, "old runtime")
	writeText(t, keep, "user content")
	plan.RemovePaths = []string{stale}
	if _, err := Apply(context.Background(), plan, ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale file still exists: %v", err)
	}
	if readText(t, keep) != "user content" {
		t.Fatal("undeclared user file changed")
	}
}

func TestApplyRejectsPayloadHashMismatchWithoutChangingTarget(t *testing.T) {
	root := t.TempDir()
	plan := testPlan(t, root, []testFile{{name: "app.exe", old: "old", next: "new"}})
	plan.Files[0].SHA256 = hashText("different")
	receipt, err := Apply(context.Background(), plan, ApplyOptions{})
	if CodeOf(err) != "HASH_MISMATCH" || receipt.ErrorCode != "HASH_MISMATCH" {
		t.Fatalf("err=%v receipt=%+v", err, receipt)
	}
	if got := readText(t, plan.Files[0].TargetPath); got != "old" {
		t.Fatalf("target changed: %q", got)
	}
}

func TestPlanRejectsTargetOutsideApplicationRoot(t *testing.T) {
	root := t.TempDir()
	plan := testPlan(t, root, []testFile{{name: "app.exe", old: "old", next: "new"}})
	plan.Files[0].TargetPath = filepath.Join(root, "outside.exe")
	if err := plan.Validate(); CodeOf(err) != "PLAN_INVALID" {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadPlanVerifiesPlanHash(t *testing.T) {
	root := t.TempDir()
	plan := testPlan(t, root, []testFile{{name: "app.exe", old: "old", next: "new"}})
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "plan.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPlan(path, hashBytes(data)); err != nil {
		t.Fatalf("LoadPlan(valid) error = %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPlan(path, hashBytes(data)); CodeOf(err) != "PLAN_HASH_MISMATCH" {
		t.Fatalf("LoadPlan(tampered) error = %v", err)
	}
}

func TestWriteReceiptCanReplaceExistingReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "receipt.json")
	if err := WriteReceipt(path, Receipt{TransactionID: "first", ProductID: "dev-helper", Version: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteReceipt(path, Receipt{TransactionID: "second", ProductID: "dev-helper", Version: "1.0.1"}); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, path); !contains(got, `"transaction_id": "second"`) {
		t.Fatalf("receipt was not replaced: %s", got)
	}
}

type testFile struct {
	name string
	old  string
	next string
}

func testPlan(t *testing.T, root string, files []testFile) Plan {
	t.Helper()
	targetRoot := filepath.Join(root, "installed")
	backupRoot := filepath.Join(root, "backup")
	sourceRoot := filepath.Join(root, "payload")
	plan := Plan{
		SchemaVersion: PlanSchemaVersion,
		TransactionID: "transaction-1",
		ProductID:     "dev-helper",
		Version:       "1.2.3",
		TargetRoot:    targetRoot,
		BackupRoot:    backupRoot,
		ReceiptPath:   filepath.Join(root, "receipts", "transaction-1.json"),
	}
	for _, item := range files {
		source := filepath.Join(sourceRoot, item.name)
		target := filepath.Join(targetRoot, item.name)
		writeText(t, source, item.next)
		writeText(t, target, item.old)
		plan.Files = append(plan.Files, FileEntry{SourcePath: source, TargetPath: target, SHA256: hashText(item.next)})
	}
	return plan
}

func writeText(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func hashText(value string) string  { return hashBytes([]byte(value)) }
func hashBytes(value []byte) string { return fmt.Sprintf("%x", sha256.Sum256(value)) }
func contains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
