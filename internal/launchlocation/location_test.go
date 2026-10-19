package launchlocation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadLauncherOutsideProductDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", filepath.Join(root, "machine"))
	executable := filepath.Join(root, "custom install", "Meow Launcher.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("launcher"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Save(executable); err != nil {
		t.Fatal(err)
	}
	record, err := Load()
	if err != nil || record.ExecutablePath != executable || record.SchemaVersion != SchemaVersion {
		t.Fatalf("record=%+v error=%v", record, err)
	}
}

func TestLoadRejectsMovedLauncher(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", filepath.Join(root, "machine"))
	executable := filepath.Join(root, "Meow Launcher.exe")
	if err := os.WriteFile(executable, []byte("launcher"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Save(executable); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(executable); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("stale Launcher location accepted")
	}
}
