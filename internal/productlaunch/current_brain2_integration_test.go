package productlaunch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"meow-launcher/internal/brain2"
)

func TestCurrentBrain2ProjectsRoundTripToHelperAndProtector(t *testing.T) {
	brainRoot := strings.TrimSpace(os.Getenv("MEOW_BRAIN2_INTEGRATION_ROOT"))
	helperRoot := strings.TrimSpace(os.Getenv("MEOW_DEV_HELPER_ROOT"))
	protectorRoot := strings.TrimSpace(os.Getenv("MEOW_PROTECTOR_DEV_ROOT"))
	deviceID := strings.TrimSpace(os.Getenv("MEOW_BRAIN2_INTEGRATION_DEVICE"))
	if brainRoot == "" || helperRoot == "" || protectorRoot == "" || deviceID == "" {
		t.Skip("set MEOW_BRAIN2_INTEGRATION_ROOT, MEOW_BRAIN2_INTEGRATION_DEVICE, MEOW_DEV_HELPER_ROOT and MEOW_PROTECTOR_DEV_ROOT")
	}
	if runtime.GOOS != "windows" {
		t.Skip("the current product runtimes are Windows-only")
	}
	config, err := brain2.LoadConfig(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	devices, err := brain2.LoadDevicesOrEmpty(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	device, exists := brain2.DeviceByID(devices, deviceID)
	if !exists {
		t.Fatalf("Brain2 device %s is not registered", deviceID)
	}
	registry, err := brain2.LoadProjects(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	if registry.SchemaVersion != brain2.ProjectRegistrySchemaVersion {
		t.Fatalf("unexpected Brain2 project registry schema %q", registry.SchemaVersion)
	}
	projects := make([]brain2.Project, 0, len(registry.Projects))
	for _, project := range registry.Projects {
		if ProjectAvailableOnDevice(project, deviceID, device.MachineName) {
			projects = append(projects, project)
		}
	}
	if len(projects) == 0 {
		t.Fatal("current Brain2 has no product-linkable project on this device")
	}

	dataRoot := filepath.Join(t.TempDir(), "launcher-data")
	preparedHelper, err := PrepareHelperLaunch(dataRoot, config.InstanceID, helperRoot, deviceID, device.MachineName, projects[0], time.Now())
	if err != nil {
		t.Fatal(err)
	}
	helperTaskData, err := os.ReadFile(preparedHelper.TaskPath)
	if err != nil {
		t.Fatal(err)
	}
	var helperTask HelperLaunchTask
	if err := json.Unmarshal(helperTaskData, &helperTask); err != nil {
		t.Fatal(err)
	}
	if helperTask.ProjectRef.RegistrySchema != registry.SchemaVersion || helperTask.ProjectRef.DeviceID != deviceID || helperTask.ProjectRef.EntityKind == "" || helperTask.ProjectRef.LifecycleStage == "" {
		t.Fatalf("Helper task is missing current Brain2 project metadata: %#v", helperTask.ProjectRef)
	}
	helperData := filepath.Join(t.TempDir(), "helper-data")
	helperUserData := filepath.Join(t.TempDir(), "helper-user-data")
	helperArgs := append([]string{"--user-data-dir=" + helperUserData}, preparedHelper.Args...)
	helperArgs = append(helperArgs, "--smoke-test", "--data-dir", helperData)
	helperCommand := exec.Command(preparedHelper.Command, helperArgs...)
	helperCommand.Dir = filepath.Clean(helperRoot)
	helperCommand.Env = os.Environ()
	if output, err := helperCommand.CombinedOutput(); err != nil {
		t.Fatalf("current Brain2 -> Helper launch failed: %v\n%s", err, output)
	}
	helperHandshake, err := AwaitHelperHandshake(preparedHelper, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if helperHandshake.RegistrySchema != registry.SchemaVersion || helperHandshake.DeviceID != deviceID {
		t.Fatalf("Helper did not bind current Brain2 identity: %#v", helperHandshake)
	}

	preparedCatalog, err := PrepareProtectorCatalogLaunch(dataRoot, config.InstanceID, protectorRoot, deviceID, device.MachineName, projects, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	catalogData, err := os.ReadFile(preparedCatalog.CatalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var catalog ProjectCatalogTask
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.RegistrySchema != registry.SchemaVersion || catalog.DeviceID != deviceID || len(catalog.Projects) != len(projects) {
		t.Fatalf("Protector catalog is missing current Brain2 identity: %#v", catalog)
	}
	protectorRuntime := filepath.Join(t.TempDir(), "protector-runtime")
	protectorSmoke := filepath.Join(t.TempDir(), "protector-smoke.json")
	protectorArgs := append(append([]string{}, preparedCatalog.Args...), "--gate-e-smoke", "--gate-e-smoke-select-project", projects[0].ID)
	protectorCommand := exec.Command(preparedCatalog.Command, protectorArgs...)
	protectorCommand.Dir = filepath.Clean(protectorRoot)
	protectorCommand.Env = append(os.Environ(), "MP_PROJECT_RUNTIME_ROOT="+protectorRuntime, "MP_GATE_E_SMOKE_OUTPUT="+protectorSmoke)
	if output, err := protectorCommand.CombinedOutput(); err != nil {
		t.Fatalf("current Brain2 -> Protector catalog failed: %v\n%s", err, output)
	}
	catalogHandshake, err := AwaitCatalogHandshake(preparedCatalog, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if catalogHandshake.RegistrySchema != registry.SchemaVersion || catalogHandshake.DeviceID != deviceID || catalogHandshake.ProjectCount != len(projects) {
		t.Fatalf("Protector did not bind current Brain2 identity: %#v", catalogHandshake)
	}
}
