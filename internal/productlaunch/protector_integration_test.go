package productlaunch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"meow-launcher/internal/brain2"
)

func TestElectronAcceptsPreparedLaunch(t *testing.T) {
	productRoot := os.Getenv("MEOW_PROTECTOR_DEV_ROOT")
	if productRoot == "" {
		t.Skip("set MEOW_PROTECTOR_DEV_ROOT to run the local cross-product integration test")
	}
	if runtime.GOOS != "windows" {
		t.Skip("the current Gate E Electron runtime is Windows-only")
	}
	base := t.TempDir()
	dataRoot := filepath.Join(base, "Launcher 产品测试")
	projectRoot := filepath.Join(base, "Brain2 已确认项目")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# controlled project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := brain2.Project{
		ID: "project-integration", Name: "Brain2 已确认项目", Status: "active", SourcePath: &projectRoot,
		PathStatus: "verified", TruthStatus: "user_confirmed",
		Assets: []brain2.ProjectAsset{{
			ID: "primary-root", Path: projectRoot, PathKind: "absolute",
			Status: "verified", DeviceID: "MACHINE-A",
		}},
	}
	prepared, err := PrepareProtectorLaunch(dataRoot, "brain2-test-workspace", productRoot, "A", "MACHINE-A", project, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(base, "protector-runtime")
	smokeOutput := filepath.Join(base, "smoke.json")
	args := append(append([]string{}, prepared.Args...), "--gate-e-smoke")
	command := exec.Command(prepared.Command, args...)
	command.Dir = filepath.Clean(productRoot)
	command.Env = append(os.Environ(), "MP_PROJECT_RUNTIME_ROOT="+runtimeRoot, "MP_GATE_E_SMOKE_OUTPUT="+smokeOutput)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Protector Electron launch failed: %v\n%s", err, output)
	}
	handshake, err := AwaitProtectorHandshake(prepared, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if handshake.TaskID != prepared.TaskID || handshake.ProjectID != prepared.ProjectID || handshake.Status != "accepted" {
		t.Fatalf("unexpected handshake: %#v", handshake)
	}
	if _, err := os.Stat(smokeOutput); err != nil {
		t.Fatalf("Protector smoke evidence missing: %v", err)
	}
}

func TestElectronAcceptsCatalogAndSelectedProject(t *testing.T) {
	productRoot := os.Getenv("MEOW_PROTECTOR_DEV_ROOT")
	if productRoot == "" {
		t.Skip("set MEOW_PROTECTOR_DEV_ROOT to run the local cross-product integration test")
	}
	if runtime.GOOS != "windows" {
		t.Skip("the current Gate E Electron runtime is Windows-only")
	}
	base := t.TempDir()
	projects := make([]brain2.Project, 0, 2)
	for index, name := range []string{"项目 A", "项目 B"} {
		root := filepath.Join(base, name)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# controlled project\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		projects = append(projects, brain2.Project{
			ID: "project-" + string(rune('a'+index)), Name: name, Status: "active", SourcePath: &root,
			PathStatus: "verified", TruthStatus: "user_confirmed",
			Assets: []brain2.ProjectAsset{{ID: "primary-root", Path: root, PathKind: "absolute", Status: "verified", DeviceID: "MACHINE-A"}},
		})
	}
	prepared, err := PrepareProtectorCatalogLaunch(filepath.Join(base, "Launcher 产品测试"), "brain2-test-workspace", productRoot, "A", "MACHINE-A", projects, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(base, "protector-runtime")
	smokeOutput := filepath.Join(base, "catalog-smoke.json")
	args := append(append([]string{}, prepared.Args...), "--gate-e-smoke", "--gate-e-smoke-select-project", "project-b")
	command := exec.Command(prepared.Command, args...)
	command.Dir = filepath.Clean(productRoot)
	command.Env = append(os.Environ(), "MP_PROJECT_RUNTIME_ROOT="+runtimeRoot, "MP_GATE_E_SMOKE_OUTPUT="+smokeOutput)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Protector catalog launch failed: %v\n%s", err, output)
	}
	handshake, err := AwaitCatalogHandshake(prepared, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if handshake.Status != "accepted" || handshake.ProjectCount != 2 {
		t.Fatalf("unexpected catalog handshake: %#v", handshake)
	}
	pointerData, err := os.ReadFile(filepath.Join(runtimeRoot, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pointer struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(pointerData, &pointer); err != nil {
		t.Fatal(err)
	}
	stateData, err := os.ReadFile(filepath.Join(runtimeRoot, pointer.State))
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Project struct {
			ProjectID string `json:"project_id"`
		} `json:"project"`
		Authorization struct {
			Confirmation string `json:"confirmation"`
		} `json:"authorization"`
	}
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatal(err)
	}
	if state.Project.ProjectID != "project-b" || state.Authorization.Confirmation != "explicit-user-read-only-scan-confirmation" {
		t.Fatalf("catalog selection did not activate the requested project: %#v", state)
	}
}
