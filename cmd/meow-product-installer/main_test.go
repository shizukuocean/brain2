package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveInitialDirectoryAllowsOnlyIsolatedCanonicalTestLayout(t *testing.T) {
	root := t.TempDir()
	machineRoot := filepath.Join(root, "machine")
	desktopRoot := filepath.Join(machineRoot, "desktop")
	canonical := filepath.Join(machineRoot, "Meow System", "Launcher")
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	t.Setenv("MEOW_DESKTOP_SHORTCUT_TEST_ROOT", desktopRoot)
	resolved, err := resolveInitialDirectory(canonical)
	if err != nil || resolved != filepath.Clean(canonical) {
		t.Fatalf("resolved=%q error=%v", resolved, err)
	}
	outside := filepath.Join(root, "outside", "Meow System", "Launcher")
	if _, err := resolveInitialDirectory(outside); err == nil {
		t.Fatal("isolated initial install accepted a path outside the machine root")
	}
}

func TestResolveInitialDirectoryUsesSelectedDriveOutsideTestMode(t *testing.T) {
	t.Setenv("MEOW_DESKTOP_SHORTCUT_TEST_ROOT", "")
	selection := t.TempDir()
	resolved, err := resolveInitialDirectory(selection)
	if err != nil {
		t.Fatal(err)
	}
	volume := filepath.VolumeName(selection)
	expected := filepath.Join(volume+string(os.PathSeparator), "Meow System", "Launcher")
	if resolved != expected {
		t.Fatalf("resolved=%q expected=%q", resolved, expected)
	}
}
