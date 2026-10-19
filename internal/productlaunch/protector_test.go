package productlaunch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meow-launcher/internal/brain2"
)

func TestPrepareProtectorLaunchBindsConfirmedProject(t *testing.T) {
	base := t.TempDir()
	dataRoot := filepath.Join(base, "launcher-data")
	projectRoot := filepath.Join(base, "测试 项目")
	productRoot := filepath.Join(base, "protector")
	for _, dir := range []string{projectRoot, filepath.Join(productRoot, "desktop"), filepath.Join(productRoot, "node_modules", "electron", "dist")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(productRoot, "package.json"), filepath.Join(productRoot, "desktop", "main.cjs"), filepath.Join(productRoot, "node_modules", "electron", "dist", "electron.exe")} {
		if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	project := brain2.Project{
		ID: "project-1", Name: "测试项目", EntityKind: "product", LifecycleStage: "active", Status: "active", SourcePath: &projectRoot,
		PathStatus: "verified", TruthStatus: "user_confirmed",
		Assets: []brain2.ProjectAsset{{
			ID: "primary-root", Path: projectRoot, PathKind: "absolute",
			Status: "verified", DeviceID: "MACHINE-A",
		}},
	}
	now := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	prepared, err := PrepareProtectorLaunch(dataRoot, "workspace-test", productRoot, "A", "MACHINE-A", project, now)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prepared.TaskPath)
	if err != nil {
		t.Fatal(err)
	}
	var task LaunchTask
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}
	if task.ProjectRef.WorkspaceID != "workspace-test" || task.ProjectRef.ProjectID != project.ID || task.ProjectRef.SourcePathSnapshot != projectRoot {
		t.Fatalf("unexpected project binding: %#v", task.ProjectRef)
	}
	if task.ProjectRef.PathStatus != "confirmed" || task.ProjectRef.TruthStatus != "user-confirmed" {
		t.Fatalf("unexpected project confirmation: %#v", task.ProjectRef)
	}
	if task.ContractVersion != ContractVersion || task.ProjectRef.RegistrySchema != brain2.ProjectRegistrySchemaVersion || task.ProjectRef.DeviceID != "A" || task.ProductGrant.DeviceID != "A" {
		t.Fatalf("current Brain2 identity was not propagated: %#v", task)
	}
	if len(task.ProductGrant.Scopes) != 3 || task.ProductGrant.Scopes[2] != "scan" || task.ProductGrant.RevokedAt != nil {
		t.Fatalf("unexpected grant: %#v", task.ProductGrant)
	}
	if task.ProductGrant.ExpiresAt <= task.ExpiresAt {
		t.Fatal("grant should outlive the short launch task")
	}
	if prepared.ReceiptPath == "" || len(prepared.Args) != 5 {
		t.Fatalf("unexpected launch command: %#v", prepared)
	}
}

func TestPrepareProtectorLaunchRejectsMismatchedAssetPath(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	project := brain2.Project{
		ID: "project-1", Status: "active", SourcePath: &root,
		PathStatus: "verified", TruthStatus: "user_confirmed",
		Assets: []brain2.ProjectAsset{{
			ID: "primary-root", Path: other, PathKind: "absolute", Status: "verified",
		}},
	}
	if _, err := PrepareProtectorLaunch(root, "workspace", root, "A", "MACHINE-A", project, time.Now()); err == nil {
		t.Fatal("mismatched primary asset path was accepted")
	}
}

func TestAwaitProtectorHandshakeAcceptsMatchingReceipt(t *testing.T) {
	root := t.TempDir()
	prepared := PreparedLaunch{
		TaskID: "plt_test", ProjectID: "project-1", DeviceID: "A",
		ReceiptPath: filepath.Join(root, "product-handshake.json"), PathFingerprint: "sha256:test",
	}
	receipt := productHandshakeReceipt{Handshake: ProductHandshake{
		ContractVersion: ContractVersion, TaskID: prepared.TaskID,
		ProductID: ProductID, ProductVersion: ProductVersion,
		RegistrySchema: brain2.ProjectRegistrySchemaVersion, DeviceID: "A",
		ProjectID: prepared.ProjectID, PathFingerprint: "sha256:test",
		AcceptedScopes: []string{"metadata", "read", "scan"}, Status: "accepted",
	}}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prepared.ReceiptPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	handshake, err := AwaitProtectorHandshake(prepared, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if handshake.Status != "accepted" || handshake.TaskID != prepared.TaskID {
		t.Fatalf("unexpected handshake: %#v", handshake)
	}
}

func TestAwaitProtectorHandshakeRejectsWrongTask(t *testing.T) {
	root := t.TempDir()
	prepared := PreparedLaunch{TaskID: "plt_expected", ProjectID: "project-1", DeviceID: "A", ReceiptPath: filepath.Join(root, "product-handshake.json"), PathFingerprint: "sha256:test"}
	receipt := productHandshakeReceipt{Handshake: ProductHandshake{
		ContractVersion: ContractVersion, TaskID: "plt_other",
		ProductID: ProductID, ProductVersion: ProductVersion,
		RegistrySchema: brain2.ProjectRegistrySchemaVersion, DeviceID: "A",
		ProjectID: prepared.ProjectID, PathFingerprint: "sha256:test",
		AcceptedScopes: []string{"metadata", "read", "scan"}, Status: "accepted",
	}}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prepared.ReceiptPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AwaitProtectorHandshake(prepared, time.Second); err == nil {
		t.Fatal("mismatched handshake task was accepted")
	}
}

func TestPrepareProtectorLaunchRejectsUnconfirmedProject(t *testing.T) {
	root := t.TempDir()
	project := brain2.Project{ID: "candidate", Status: "active", SourcePath: &root, PathStatus: "candidate", TruthStatus: "agent_suggested"}
	if _, err := PrepareProtectorLaunch(root, "workspace", root, "A", "MACHINE-A", project, time.Now()); err == nil {
		t.Fatal("unconfirmed project was accepted")
	}
}
