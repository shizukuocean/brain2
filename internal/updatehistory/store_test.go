package updatehistory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAndReconcileSuccessfulReceiptAdvancesInstalledVersion(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	receiptPath := filepath.Join(root, "receipts", "tx-1.json")
	executablePath := filepath.Join(root, "installed", "Dev Helper.exe")
	state, err := RecordAttempt(statePath, ProductRecord{
		ProductID: "dev-helper", AttemptedVersion: "1.2.3", TransactionID: "tx-1",
		ReceiptPath: receiptPath, ExecutablePath: executablePath,
	})
	if err != nil || state.Products["dev-helper"].Status != "starting" {
		t.Fatalf("state=%+v error=%v", state, err)
	}
	if _, err := RecordInstallerStarted(statePath, "dev-helper", "tx-1", 1234, "installer_started"); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, receiptPath, Receipt{
		SchemaVersion: ReceiptSchemaVersion, TransactionID: "tx-1", ProductID: "dev-helper", Version: "1.2.3",
		Status: "installed", Message: "installed", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: executablePath,
	})
	state, err = ReconcilePending(statePath)
	if err != nil {
		t.Fatal(err)
	}
	record := state.Products["dev-helper"]
	if record.Status != "installed" || record.InstalledVersion != "1.2.3" || record.ExecutablePath != executablePath || record.InstallerPID != 1234 {
		t.Fatalf("record=%+v", record)
	}
}

func TestFailedReceiptPreservesPreviouslyInstalledVersion(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	receiptPath := filepath.Join(root, "receipt.json")
	if _, err := RecordAttempt(statePath, ProductRecord{ProductID: "brain2-manager", AttemptedVersion: "1.0.0", TransactionID: "tx-old", ReceiptPath: filepath.Join(root, "old.json")}); err != nil {
		t.Fatal(err)
	}
	state, err := Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	record := state.Products["brain2-manager"]
	record.InstalledVersion = "0.9.0"
	state.Products["brain2-manager"] = record
	if err := save(statePath, state); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordAttempt(statePath, ProductRecord{ProductID: "brain2-manager", AttemptedVersion: "1.0.0", TransactionID: "tx-new", ReceiptPath: receiptPath}); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, receiptPath, Receipt{SchemaVersion: ReceiptSchemaVersion, TransactionID: "tx-new", ProductID: "brain2-manager", Version: "1.0.0", Status: "failed", ErrorCode: "INSTALL_FAILED", Message: "rolled back", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	state, err = ReconcilePending(statePath)
	if err != nil {
		t.Fatal(err)
	}
	record = state.Products["brain2-manager"]
	if record.Status != "failed" || record.InstalledVersion != "0.9.0" || record.ErrorCode != "INSTALL_FAILED" {
		t.Fatalf("record=%+v", record)
	}
}

func TestMismatchedReceiptNeverAdvancesVersion(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	receiptPath := filepath.Join(root, "receipt.json")
	if _, err := RecordAttempt(statePath, ProductRecord{ProductID: "dev-helper", AttemptedVersion: "1.2.3", TransactionID: "expected", ReceiptPath: receiptPath}); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, receiptPath, Receipt{SchemaVersion: ReceiptSchemaVersion, TransactionID: "other", ProductID: "dev-helper", Version: "1.2.3", Status: "installed", Message: "spoofed", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	state, err := ReconcilePending(statePath)
	if err != nil {
		t.Fatal(err)
	}
	record := state.Products["dev-helper"]
	if record.Status != "receipt_invalid" || record.ErrorCode != "RECEIPT_IDENTITY_MISMATCH" || record.InstalledVersion != "" {
		t.Fatalf("record=%+v", record)
	}
}

func TestLegacyLauncherAcceptsCanonicalMigrationReceiptFromNewInstaller(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	receiptPath := filepath.Join(root, "receipt.json")
	legacyExecutable := filepath.Join(root, "legacy-launcher", "Meow Launcher.exe")
	canonicalExecutable := filepath.Join(root, "Meow System", "Launcher", "Meow Launcher.exe")
	if _, err := RecordAttempt(statePath, ProductRecord{
		ProductID: "meow-launcher", AttemptedVersion: "0.6.7-preview.30", TransactionID: "tx-migrate",
		ReceiptPath: receiptPath, ExecutablePath: legacyExecutable,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordInstallerStarted(statePath, "meow-launcher", "tx-migrate", 3030, "self_update_installer_started"); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, receiptPath, Receipt{
		SchemaVersion: ReceiptSchemaVersion, TransactionID: "tx-migrate", ProductID: "meow-launcher", Version: "0.6.7-preview.30",
		Status: "installed", Message: "installed", Restarted: true, CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
		ExecutablePath: canonicalExecutable,
	})
	state, err := ReconcilePending(statePath)
	if err != nil {
		t.Fatal(err)
	}
	record := state.Products["meow-launcher"]
	if record.Status != "installed" || record.InstalledVersion != "0.6.7-preview.30" ||
		record.ExecutablePath != canonicalExecutable || record.PreviousExecutablePath != legacyExecutable ||
		record.MigrationStatus != "pending" || !record.ReceiptRestarted {
		t.Fatalf("record=%+v", record)
	}
}

func TestFailedLegacyLauncherMigrationReceiptKeepsOldExecutableAuthoritative(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	receiptPath := filepath.Join(root, "receipt.json")
	legacyExecutable := filepath.Join(root, "legacy-launcher", "Meow Launcher.exe")
	canonicalExecutable := filepath.Join(root, "Meow System", "Launcher", "Meow Launcher.exe")
	if _, err := RecordAttempt(statePath, ProductRecord{
		ProductID: "meow-launcher", AttemptedVersion: "0.6.7-preview.30", TransactionID: "tx-failed-migrate",
		ReceiptPath: receiptPath, ExecutablePath: legacyExecutable,
	}); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, receiptPath, Receipt{
		SchemaVersion: ReceiptSchemaVersion, TransactionID: "tx-failed-migrate", ProductID: "meow-launcher", Version: "0.6.7-preview.30",
		Status: "failed", ErrorCode: "INSTALL_FAILED", Message: "rolled back", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
		ExecutablePath: canonicalExecutable,
	})
	state, err := ReconcilePending(statePath)
	if err != nil {
		t.Fatal(err)
	}
	record := state.Products["meow-launcher"]
	if record.Status != "failed" || record.ExecutablePath != legacyExecutable || record.PreviousExecutablePath != "" || record.MigrationStatus != "" {
		t.Fatalf("record=%+v", record)
	}
}

func TestMissingReceiptKeepsAttemptPendingWithoutRewritingState(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	if _, err := RecordAttempt(statePath, ProductRecord{ProductID: "dev-helper", AttemptedVersion: "1.2.3", TransactionID: "tx", ReceiptPath: filepath.Join(root, "missing.json")}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	state, err := ReconcilePending(statePath)
	if err != nil || state.Products["dev-helper"].Status != "starting" {
		t.Fatalf("state=%+v error=%v", state, err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("missing receipt unexpectedly rewrote update state")
	}
}

func TestInstallerStartFailureIsRecordedWithoutAdvancingVersion(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	if _, err := RecordAttempt(statePath, ProductRecord{ProductID: "dev-helper", AttemptedVersion: "1.2.3", TransactionID: "tx", ReceiptPath: filepath.Join(root, "receipt.json")}); err != nil {
		t.Fatal(err)
	}
	state, err := RecordAttemptFailed(statePath, "dev-helper", "tx", "INSTALLER_START_FAILED", "could not start")
	if err != nil {
		t.Fatal(err)
	}
	record := state.Products["dev-helper"]
	if record.Status != "install_start_failed" || record.ErrorCode != "INSTALLER_START_FAILED" || record.InstalledVersion != "" {
		t.Fatalf("record=%+v", record)
	}
}

func TestReceiptSymlinkIsRejected(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "real.json")
	linkPath := filepath.Join(root, "link.json")
	writeReceipt(t, realPath, Receipt{SchemaVersion: ReceiptSchemaVersion, TransactionID: "tx", ProductID: "dev-helper", Version: "1.2.3", Status: "installed", Message: "ok", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	statePath := filepath.Join(root, "state.json")
	if _, err := RecordAttempt(statePath, ProductRecord{ProductID: "dev-helper", AttemptedVersion: "1.2.3", TransactionID: "tx", ReceiptPath: linkPath}); err != nil {
		t.Fatal(err)
	}
	state, err := ReconcilePending(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.Products["dev-helper"].Status != "receipt_invalid" {
		t.Fatalf("record=%+v", state.Products["dev-helper"])
	}
}

func TestRemoveProductClearsOnlySelectedInstallRecord(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	for _, productID := range []string{"dev-helper", "brain2-manager"} {
		if _, err := RecordAttempt(statePath, ProductRecord{
			ProductID: productID, AttemptedVersion: "1.0.0", TransactionID: "tx-" + productID, ReceiptPath: filepath.Join(root, productID+".json"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	state, err := RemoveProduct(statePath, "dev-helper")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Products["dev-helper"]; ok || state.Products["brain2-manager"].ProductID != "brain2-manager" {
		t.Fatalf("state=%+v", state.Products)
	}
}

func writeReceipt(t *testing.T, path string, receipt Receipt) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
