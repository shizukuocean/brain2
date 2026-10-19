package productlaunch

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestDevHelperElectronAcceptsPreparedLaunch(t *testing.T) {
	productRoot := os.Getenv("MEOW_DEV_HELPER_ROOT")
	if productRoot == "" {
		t.Skip("set MEOW_DEV_HELPER_ROOT to run the local Dev Helper integration test")
	}
	if runtime.GOOS != "windows" {
		t.Skip("the current Dev Helper Electron runtime is Windows-only")
	}
	dataRoot, projectRoot, _ := helperTestRoots(t)
	prepared, err := PrepareHelperLaunch(dataRoot, "brain2-test-workspace", productRoot, "A", "MACHINE-A", confirmedHelperProject(projectRoot), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	helperData := filepath.Join(t.TempDir(), "helper-data")
	userData := filepath.Join(t.TempDir(), "electron-user-data")
	args := append([]string{"--user-data-dir=" + userData}, prepared.Args...)
	args = append(args, "--smoke-test", "--data-dir", helperData)
	command := exec.Command(prepared.Command, args...)
	command.Dir = filepath.Clean(productRoot)
	command.Env = os.Environ()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Dev Helper Electron launch failed: %v\n%s", err, output)
	}
	handshake, err := AwaitHelperHandshake(prepared, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if handshake.TaskID != prepared.TaskID || handshake.ProjectID != prepared.ProjectID || handshake.Status != "accepted" {
		t.Fatalf("unexpected Dev Helper handshake: %#v", handshake)
	}
}
