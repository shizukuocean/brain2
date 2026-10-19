package brain2

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeWorkspaceAndRejectNonEmptyTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2 中文")
	if err := Initialize(root); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if !IsWorkspace(root) {
		t.Fatal("initialized root was not recognized as a workspace")
	}
	cfg, err := EnsureInstanceID(root)
	if err != nil || cfg.InstanceID == "" {
		t.Fatalf("workspace instance id missing: cfg=%#v err=%v", cfg, err)
	}
	for _, rel := range []string{
		"NAV.md",
		"AGENT_START_HERE.md",
		filepath.Join(".brain2", "config.json"),
		filepath.Join(".brain2", "projects.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("required file %s missing: %v", rel, err)
		}
	}

	nonEmpty := filepath.Join(t.TempDir(), "not-empty")
	if err := os.MkdirAll(nonEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(nonEmpty, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(nonEmpty); err == nil {
		t.Fatal("expected non-empty target to be rejected")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("existing data was changed: data=%q err=%v", data, err)
	}
}

func TestAttachExistingPreservesLegacyBrain2Content(t *testing.T) {
	root := filepath.Join(t.TempDir(), "legacy-brain2")
	legacyFiles := map[string]string{
		"NAV.md": "# Existing navigation\n\nDo not replace me.\n",
		filepath.Join("06 AI助理协同", "START_HERE.md"):   "# Existing start\n",
		filepath.Join("06 AI助理协同", "规则", "大脑治理总则.md"): "# Existing governance\n",
	}
	for rel, content := range legacyFiles {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := AttachExisting(root); err != nil {
		t.Fatalf("AttachExisting returned error: %v", err)
	}
	if !IsWorkspace(root) {
		t.Fatal("attached legacy Brain2 was not recognized as a workspace")
	}
	for rel, want := range legacyFiles {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if got := string(data); got != want {
			t.Fatalf("legacy file %s was modified: got %q want %q", rel, got, want)
		}
	}
	for _, rel := range []string{
		filepath.Join(".brain2", "config.json"),
		filepath.Join(".brain2", "projects.json"),
		filepath.Join(".brain2", "agents.json"),
		filepath.Join(".brain2", "version.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("attached metadata file %s missing: %v", rel, err)
		}
	}
}

func TestAttachOrdinaryObsidianVaultAddsOnlyMachineFramework(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ordinary-vault")
	if err := os.MkdirAll(filepath.Join(root, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(root, "My Note.md")
	want := "# My Note\n\nKeep exactly as written.\n"
	if err := os.WriteFile(note, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AttachExisting(root); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(note); err != nil || string(data) != want {
		t.Fatalf("existing vault content changed: data=%q err=%v", data, err)
	}
	for _, rel := range managedBootstrapFiles() {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("managed bootstrap %s missing: %v", rel, err)
		}
	}
	for _, unexpected := range []string{"00 收件箱", "02 项目", "06 AI助理协同"} {
		if _, err := os.Stat(filepath.Join(root, unexpected)); !os.IsNotExist(err) {
			t.Fatalf("ordinary vault received content-layer structure %q: %v", unexpected, err)
		}
	}
	report, err := Check(root)
	if err != nil || report.OverallStatus == "critical" {
		t.Fatalf("ordinary vault machine framework must be usable: report=%#v err=%v", report, err)
	}
}

func TestWorkspaceRootIdentityTreatsWindowsAndWSLMountAsSameWorkspace(t *testing.T) {
	windowsIdentity, windowsOK := workspaceRootIdentity(`D:\MeowLauncher\acceptance\brain2-p0-test`)
	wslIdentity, wslOK := workspaceRootIdentity(`/mnt/d/MeowLauncher/acceptance/brain2-p0-test`)
	if !windowsOK || !wslOK || windowsIdentity != wslIdentity {
		t.Fatalf("root identities differ: windows=%q wsl=%q", windowsIdentity, wslIdentity)
	}
}

func TestAttachExistingRejectsUnknownConfig(t *testing.T) {
	root := filepath.Join(t.TempDir(), "legacy-brain2")
	for rel, content := range map[string]string{
		"NAV.md": "keep",
		filepath.Join("06 AI助理协同", "START_HERE.md"):   "keep",
		filepath.Join("06 AI助理协同", "规则", "大脑治理总则.md"): "keep",
		filepath.Join(".brain2", "config.json"):       "{\"owned_by\":\"another-tool\"}\n",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := AttachExisting(root); err == nil {
		t.Fatal("expected unknown existing config to be rejected")
	}
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "{\"owned_by\":\"another-tool\"}\n"; got != want {
		t.Fatalf("unknown config was modified: got %q want %q", got, want)
	}
}

func TestCreateAndAttachProjects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := CreateInternalProject(root, "内部项目", "跑通项目闭环"); err != nil {
		t.Fatalf("CreateInternalProject returned error: %v", err)
	}
	if err := CreateInternalProject(root, "内部项目", "重复"); err == nil {
		t.Fatal("expected duplicate internal project to be rejected")
	}

	external := filepath.Join(t.TempDir(), "外部源码")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(external, "source.txt")
	if err := os.WriteFile(marker, []byte("do-not-move"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AttachExternalProject(root, "外部项目", external, "只登记，不移动"); err != nil {
		t.Fatalf("AttachExternalProject returned error: %v", err)
	}
	if err := AttachExternalProject(root, "外部项目", external, "重复"); err == nil {
		t.Fatal("expected duplicate external project to be rejected")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "do-not-move" {
		t.Fatalf("external source was changed: data=%q err=%v", data, err)
	}

	registry, err := LoadProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(registry.Projects))
	}
	if registry.SchemaVersion != ProjectRegistrySchemaVersion {
		t.Fatalf("unexpected project registry version: %q", registry.SchemaVersion)
	}
	if registry.Projects[1].SourcePath == nil || *registry.Projects[1].SourcePath != filepath.Clean(external) {
		t.Fatalf("unexpected external source path: %#v", registry.Projects[1].SourcePath)
	}
	for _, project := range registry.Projects {
		if project.ManifestVersion != projectManifestVersion || project.ManifestPath == "" {
			t.Fatalf("project identity was not registered: %#v", project)
		}
		if project.AssetStatus != "registered" || project.PathStatus != "verified" || project.TruthStatus != "user_confirmed" {
			t.Fatalf("unexpected project identity status: %#v", project)
		}
		if project.ConnectionGate != "user_confirmation_required" {
			t.Fatalf("downstream product gate missing: %#v", project)
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(project.ManifestPath)))
		if err != nil {
			t.Fatalf("manifest missing for %s: %v", project.ID, err)
		}
		values := readManifestScalars(string(data))
		if values["project_id"] != project.ID || values["project_name"] != project.Name || values["brain2_project_manifest"] != projectManifestVersion {
			t.Fatalf("manifest identity mismatch for %s: %#v", project.ID, values)
		}
	}
	report, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OverallStatus != "healthy" || report.Score != 100 {
		t.Fatalf("managed project manifests should be healthy: %#v", report)
	}
}

func TestLegacyProjectRegistryRemainsReadableAndNeedsIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	legacy := ProjectRegistry{SchemaVersion: "0.1", Projects: []Project{{
		ID: "legacy", Name: "旧项目", Mode: "internal", Brain2Entry: "02 项目/旧项目/README.md",
		Status: "active", Goal: "保留", Tags: []string{}, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}}}
	if err := os.MkdirAll(filepath.Join(root, "02 项目", "旧项目"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "02 项目", "旧项目", "README.md"), []byte("# 旧项目\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), legacy); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadProjects(root)
	if err != nil || len(loaded.Projects) != 1 || loaded.Projects[0].ManifestPath != "" {
		t.Fatalf("legacy registry compatibility failed: registry=%#v err=%v", loaded, err)
	}
	report, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OverallStatus != "needs_attention" || report.Score != 95 {
		t.Fatalf("legacy project should request identity without becoming critical: %#v", report)
	}
}

func TestLegacyInternalProjectCanBeConfirmedWithoutGuessing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	legacy := ProjectRegistry{SchemaVersion: "0.1", Projects: []Project{{
		ID: "legacy", Name: "旧项目", Mode: "internal", Brain2Entry: "02 项目/旧项目/README.md",
		Status: "active", Goal: "保留", Tags: []string{}, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z",
	}}}
	entryDir := filepath.Join(root, "02 项目", "旧项目")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entryDir, "README.md"), []byte("# 旧项目\n\n保留原正文。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), legacy); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmProjectManifest(root, "legacy", ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	project := loaded.Projects[0]
	if loaded.SchemaVersion != ProjectRegistrySchemaVersion || project.ManifestPath == "" || project.PathStatus != "verified" || project.TruthStatus != "user_confirmed" {
		t.Fatalf("legacy identity not confirmed: %#v", loaded)
	}
	readme, err := os.ReadFile(filepath.Join(entryDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "保留原正文。") || !strings.Contains(string(readme), "brain2-project-identity:start") {
		t.Fatalf("legacy README was overwritten or not linked: %s", readme)
	}
	report, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OverallStatus != "healthy" || report.Score != 100 {
		t.Fatalf("confirmed legacy project should be healthy: %#v", report)
	}
}

func TestLegacyExternalProjectRequiresConfirmedRelocation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	oldSource := filepath.Join(t.TempDir(), "missing-source")
	legacy := ProjectRegistry{SchemaVersion: "0.1", Projects: []Project{{
		ID: "external", Name: "旧外部项目", Mode: "external", SourcePath: &oldSource,
		Brain2Entry: "02 项目/旧外部项目/README.md", Status: "active", Tags: []string{},
	}}}
	entryDir := filepath.Join(root, "02 项目", "旧外部项目")
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entryDir, "README.md"), []byte("# 旧外部项目\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), legacy); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmProjectManifest(root, "external", ""); err == nil {
		t.Fatal("missing external path must require an explicit relocation")
	}
	confirmed := filepath.Join(t.TempDir(), "confirmed-source")
	if err := os.MkdirAll(confirmed, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(confirmed, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmProjectManifest(root, "external", confirmed); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	project := loaded.Projects[0]
	if project.SourcePath == nil || *project.SourcePath != filepath.Clean(confirmed) || project.ManifestPath == "" {
		t.Fatalf("external relocation not persisted: %#v", project)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("external source changed during relocation: data=%q err=%v", data, err)
	}
}

func TestConfirmProjectManifestForDevicePreservesOtherDeviceAsset(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	entry := "02 项目/跨设备项目/README.md"
	entryPath := filepath.Join(root, filepath.FromSlash(entry))
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryPath, []byte("# 跨设备项目\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	aPath := `E:\\Projects\\shared-project`
	registry := ProjectRegistry{SchemaVersion: ProjectRegistrySchemaVersion, Projects: []Project{{
		ID: "shared-project", Name: "跨设备项目", Mode: "external", SourcePath: &aPath,
		Brain2Entry: entry, Status: "active", RepositoryURL: "https://example.invalid/shared-project",
		Assets: []ProjectAsset{{
			ID: "primary-root", Role: "source_root", Path: aPath, PathKind: "absolute",
			DeviceID: "OLD-MACHINE", Status: "verified", Truth: "current", Access: "metadata_only",
		}},
	}}}
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), registry); err != nil {
		t.Fatal(err)
	}
	bPath := filepath.Join(t.TempDir(), "shared-project-b")
	if err := os.MkdirAll(bPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmProjectManifestForDevice(root, "shared-project", bPath, "B"); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	project := loaded.Projects[0]
	if project.SourcePath == nil || *project.SourcePath != aPath {
		t.Fatalf("device B relocation replaced device A source path: %#v", project.SourcePath)
	}
	paths := map[string]string{}
	for _, asset := range project.Assets {
		if asset.PathKind == "absolute" {
			paths[asset.DeviceID] = asset.Path
		}
	}
	if paths["A"] != aPath || paths["B"] != filepath.Clean(bPath) {
		t.Fatalf("expected independent A/B assets, got %#v", paths)
	}
}

func TestConfirmProjectManifestRejectsEntryOutsideBrain2(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	legacy := ProjectRegistry{SchemaVersion: "0.1", Projects: []Project{{
		ID: "escape", Name: "越界项目", Mode: "internal", Brain2Entry: "../outside/README.md", Status: "active",
	}}}
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), legacy); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmProjectManifest(root, "escape", ""); err == nil || !strings.Contains(err.Error(), "超出 Brain2") {
		t.Fatalf("expected governed path rejection, got %v", err)
	}
}

func TestProjectManifestMismatchIsReported(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := CreateInternalProject(root, "受管项目", "验证一致性"); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, filepath.FromSlash(registry.Projects[0].ManifestPath))
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(data), `project_id: "受管项目"`, `project_id: "wrong-id"`, 1)
	if changed == string(data) {
		t.Fatal("test fixture did not change project id")
	}
	if err := os.WriteFile(manifest, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OverallStatus != "needs_attention" || report.Score != 90 {
		t.Fatalf("manifest mismatch should be reported: %#v", report)
	}
	found := false
	for _, check := range report.Checks {
		if check.Name == "project_manifest:受管项目" && check.Status == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("manifest mismatch check missing: %#v", report.Checks)
	}
}

func TestPrepareCodexRuntimeAndHealth(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntime(root, "codex"); err != nil {
		t.Fatalf("PrepareRuntime returned error: %v", err)
	}
	for _, rel := range []string{
		filepath.Join(".brain2", "runtime", "AGENT_START_HERE.md"),
		filepath.Join(".brain2", "runtime", "FIRST_HANDSHAKE.md"),
		filepath.Join(".brain2", "runtime", "active-protocol.md"),
		filepath.Join(".brain2", "runtime", "current-adapter.md"),
		filepath.Join(".brain2", "runtime", "agent-config.json"),
		filepath.Join(".brain2", "runtime", "runtime-version.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("runtime file %s missing: %v", rel, err)
		}
	}

	agentsData, err := os.ReadFile(filepath.Join(root, ".brain2", "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	var agents AgentRegistry
	if err := json.Unmarshal(agentsData, &agents); err != nil {
		t.Fatal(err)
	}
	if len(agents.Agents) != 1 || agents.Agents[0].ID != "codex" {
		t.Fatalf("unexpected agent registry: %#v", agents)
	}
	if agents.Agents[0].AdapterLoaded || agents.Agents[0].ConnectionStatus != string(ConnectionPrepared) {
		t.Fatalf("prepared runtime must not be reported as connected: %#v", agents.Agents[0])
	}

	report, err := Check(root)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if report.OverallStatus != "healthy" || report.Score != 100 {
		t.Fatalf("unexpected health report: %#v", report)
	}
}

func TestHealthRejectsNonWorkspace(t *testing.T) {
	if _, err := Check(t.TempDir()); err == nil {
		t.Fatal("expected non-workspace health check to fail")
	}
}
