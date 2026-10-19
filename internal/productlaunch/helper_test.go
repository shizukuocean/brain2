package productlaunch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meow-launcher/internal/brain2"
)

func helperTestRoots(t *testing.T) (string, string, string) {
	t.Helper()
	base := t.TempDir()
	dataRoot := filepath.Join(base, "launcher-data")
	projectRoot := filepath.Join(base, "project")
	productRoot := filepath.Join(base, "helper")
	for _, dir := range []string{projectRoot, filepath.Join(productRoot, "desktop"), filepath.Join(productRoot, "node_modules", "electron", "dist")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(productRoot, "package.json"), filepath.Join(productRoot, "desktop", "main.js"), filepath.Join(productRoot, "node_modules", "electron", "dist", "electron.exe")} {
		if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dataRoot, projectRoot, productRoot
}

func confirmedHelperProject(projectRoot string) brain2.Project {
	return brain2.Project{
		ID: "project-1", Name: "测试项目", EntityKind: "product", LifecycleStage: "active",
		RepositoryURL: "https://github.com/example/project", DefaultBranch: "main",
		Status: "active", SourcePath: &projectRoot,
		PathStatus: "verified", TruthStatus: "user_confirmed",
		Assets: []brain2.ProjectAsset{{
			ID: "primary-root", Path: projectRoot, PathKind: "absolute", Status: "verified", DeviceID: "MACHINE-A",
		}},
	}
}

func TestPrepareHelperLaunchBindsOnlyConfirmedProject(t *testing.T) {
	dataRoot, projectRoot, productRoot := helperTestRoots(t)
	prepared, err := PrepareHelperLaunch(dataRoot, "workspace-test", productRoot, "A", "MACHINE-A", confirmedHelperProject(projectRoot), time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prepared.TaskPath)
	if err != nil {
		t.Fatal(err)
	}
	var task HelperLaunchTask
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}
	if task.ProductID != HelperProductID || task.ProductVersion != HelperProductVersion || task.ProjectRef.ProjectID != "project-1" {
		t.Fatalf("unexpected helper task identity: %#v", task)
	}
	if !sameScopes(task.RequestedScopes, []string{"metadata", "read", "candidate_write"}) || !sameScopes(task.ProductGrant.Scopes, task.RequestedScopes) {
		t.Fatalf("unexpected helper scopes: %#v", task)
	}
	if task.ProjectRef.SourcePathSnapshot != projectRoot || task.ProjectRef.PathFingerprint == "" || task.ProductGrant.RevokedAt != nil {
		t.Fatalf("unexpected helper project binding: %#v", task)
	}
	if task.ContractVersion != ContractVersion || task.ProjectRef.RegistrySchema != brain2.ProjectRegistrySchemaVersion || task.ProjectRef.DeviceID != "A" || task.ProductGrant.DeviceID != "A" {
		t.Fatalf("current Brain2 identity was not propagated: %#v", task)
	}
}

func TestPrepareHelperLaunchRejectsUnconfirmedAndMismatchedProject(t *testing.T) {
	dataRoot, projectRoot, productRoot := helperTestRoots(t)
	project := confirmedHelperProject(projectRoot)
	project.TruthStatus = "agent_suggested"
	if _, err := PrepareHelperLaunch(dataRoot, "workspace-test", productRoot, "A", "MACHINE-A", project, time.Now()); err == nil {
		t.Fatal("unconfirmed project was accepted")
	}
	project = confirmedHelperProject(projectRoot)
	project.Assets[0].Path = t.TempDir()
	project.Assets[0].DeviceID = "MACHINE-B"
	if _, err := PrepareHelperLaunch(dataRoot, "workspace-test", productRoot, "A", "MACHINE-A", project, time.Now()); err == nil {
		t.Fatal("asset from another device was accepted")
	}
}

func TestAwaitHelperHandshakeRequiresExactIdentityAndScopes(t *testing.T) {
	root := t.TempDir()
	prepared := PreparedHelperLaunch{
		TaskID: "hlt_test", ProjectID: "project-1", DeviceID: "A", ReceiptPath: filepath.Join(root, "receipt.json"),
		PathFingerprint: "sha256:test", RequestedScopes: []string{"metadata", "read", "candidate_write"},
	}
	writeReceipt := func(handshake HelperProductHandshake) {
		t.Helper()
		data, err := json.Marshal(helperHandshakeReceipt{Handshake: handshake})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(prepared.ReceiptPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	valid := HelperProductHandshake{
		ContractVersion: HelperContractVersion, TaskID: prepared.TaskID,
		ProductID: HelperProductID, ProductVersion: HelperProductVersion,
		RegistrySchema: brain2.ProjectRegistrySchemaVersion, DeviceID: "A",
		ProjectID: prepared.ProjectID, PathFingerprint: prepared.PathFingerprint,
		AcceptedScopes: append([]string(nil), prepared.RequestedScopes...), Status: "accepted",
	}
	writeReceipt(valid)
	if _, err := AwaitHelperHandshake(prepared, time.Second); err != nil {
		t.Fatal(err)
	}
	valid.AcceptedScopes = []string{"metadata", "read"}
	writeReceipt(valid)
	if _, err := AwaitHelperHandshake(prepared, time.Second); err == nil {
		t.Fatal("partial Helper scopes were accepted")
	}
	valid.AcceptedScopes = append([]string(nil), prepared.RequestedScopes...)
	valid.ProductID = ProductID
	writeReceipt(valid)
	if _, err := AwaitHelperHandshake(prepared, time.Second); err == nil {
		t.Fatal("Protector identity was accepted as Helper")
	}
}
