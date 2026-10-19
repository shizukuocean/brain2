package productlaunch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meow-launcher/internal/brain2"
)

func catalogTestProduct(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "protector")
	for _, dir := range []string{filepath.Join(root, "desktop"), filepath.Join(root, "node_modules", "electron", "dist")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(root, "package.json"), filepath.Join(root, "desktop", "main.cjs"), filepath.Join(root, "node_modules", "electron", "dist", "electron.exe")} {
		if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func catalogTestProject(t *testing.T, id, name string) brain2.Project {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return brain2.Project{
		ID: id, Name: name, EntityKind: "product", LifecycleStage: "active", Status: "active", SourcePath: &root,
		PathStatus: "verified", TruthStatus: "user_confirmed",
		Assets: []brain2.ProjectAsset{{ID: "primary-root", Path: root, PathKind: "absolute", Status: "verified", DeviceID: "MACHINE-A"}},
	}
}

func TestPrepareProtectorCatalogContainsOnlyProvidedConfirmedProjects(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "launcher-data")
	projects := []brain2.Project{
		catalogTestProject(t, "project-b", "项目 B"),
		catalogTestProject(t, "project-a", "项目 A"),
	}
	prepared, err := PrepareProtectorCatalogLaunch(dataRoot, "workspace-test", catalogTestProduct(t), "A", "MACHINE-A", projects, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prepared.CatalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var catalog ProjectCatalogTask
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	if prepared.ProjectCount != 2 || len(catalog.Projects) != 2 {
		t.Fatalf("unexpected catalog size: %#v", catalog)
	}
	if catalog.Projects[0].DisplayName != "项目 A" || catalog.Projects[1].DisplayName != "项目 B" {
		t.Fatalf("catalog was not sorted for user selection: %#v", catalog.Projects)
	}
	for _, entry := range catalog.Projects {
		if entry.LaunchTask.ProjectRef.ProjectID != entry.ProjectID || entry.LaunchTask.ProductGrant.ProjectID != entry.ProjectID {
			t.Fatalf("catalog entry identity mismatch: %#v", entry)
		}
	}
}

func TestPrepareProtectorCatalogRejectsUnconfirmedProject(t *testing.T) {
	project := catalogTestProject(t, "candidate", "候选")
	project.TruthStatus = "agent_suggested"
	if _, err := PrepareProtectorCatalogLaunch(t.TempDir(), "workspace-test", catalogTestProduct(t), "A", "MACHINE-A", []brain2.Project{project}, time.Now()); err == nil {
		t.Fatal("unconfirmed project entered the Protector catalog")
	}
}

func TestAwaitCatalogHandshakeRejectsMismatchedCount(t *testing.T) {
	root := t.TempDir()
	prepared := PreparedCatalogLaunch{CatalogID: "pcat_test", DeviceID: "A", ProjectCount: 2, ReceiptPath: filepath.Join(root, "catalog-handshake.json")}
	receipt := catalogHandshakeReceipt{Handshake: CatalogHandshake{
		ContractVersion: CatalogContractVersion, CatalogID: prepared.CatalogID,
		ProductID: ProductID, ProductVersion: ProductVersion,
		RegistrySchema: brain2.ProjectRegistrySchemaVersion, DeviceID: "A",
		ProjectCount: 1, Status: "accepted",
	}}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prepared.ReceiptPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AwaitCatalogHandshake(prepared, time.Second); err == nil {
		t.Fatal("mismatched project count was accepted")
	}
}
