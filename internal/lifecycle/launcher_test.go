package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocateLauncherUsesExplicitAbsolutePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Meow Launcher.exe")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_LAUNCHER_PATH", path)
	got, err := LocateLauncher()
	if err != nil || got != path {
		t.Fatalf("LocateLauncher() = %q, %v", got, err)
	}
}

func TestLocateLauncherReportsMissingExplicitPath(t *testing.T) {
	t.Setenv("MEOW_LAUNCHER_PATH", filepath.Join(t.TempDir(), "missing.exe"))
	if _, err := LocateLauncher(); err == nil {
		t.Fatal("expected missing launcher error")
	}
}
