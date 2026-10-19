//go:build windows

package desktopshortcut

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCreatesUpdatesAndRemovesIsolatedDesktopShortcut(t *testing.T) {
	root := t.TempDir()
	machineRoot := filepath.Join(root, "machine")
	desktopRoot := filepath.Join(machineRoot, "desktop")
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	t.Setenv(testDesktopRootEnvironment, desktopRoot)

	firstTarget := filepath.Join(root, "old", "Dev Helper.exe")
	secondTarget := filepath.Join(root, "Meow System", "Helper", "Dev Helper.exe")
	for _, target := range []string{firstTarget, secondTarget} {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("test executable"), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	shortcutPath, err := Ensure(Request{TargetExecutable: firstTarget, Description: "打开 Dev Helper"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(shortcutPath, filepath.Join(desktopRoot, "Dev Helper.lnk")) {
		t.Fatalf("shortcut path = %q", shortcutPath)
	}
	if target, err := readShortcutTarget(shortcutPath); err != nil || !strings.EqualFold(filepath.Clean(target), firstTarget) {
		t.Fatalf("first shortcut target=%q error=%v", target, err)
	}

	if _, err := Ensure(Request{TargetExecutable: secondTarget, Description: "打开 Dev Helper"}); err != nil {
		t.Fatal(err)
	}
	if target, err := readShortcutTarget(shortcutPath); err != nil || !strings.EqualFold(filepath.Clean(target), secondTarget) {
		t.Fatalf("updated shortcut target=%q error=%v", target, err)
	}
	if err := RemoveIfTarget(secondTarget); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(shortcutPath); !os.IsNotExist(err) {
		t.Fatalf("shortcut remains after uninstall cleanup: %v", err)
	}
}

func TestEnsurePreservesSameNamedShortcutForAnotherProgram(t *testing.T) {
	root := t.TempDir()
	existingTarget := filepath.Join(root, "other", "Other App.exe")
	requestedTarget := filepath.Join(root, "Meow System", "Launcher", "Meow Launcher.exe")
	shortcutPath := filepath.Join(root, "Meow Launcher.lnk")
	if err := writeShortcut(shortcutPath, existingTarget, filepath.Dir(existingTarget), existingTarget+",0", "other"); err != nil {
		t.Fatal(err)
	}
	if err := ensureAt(shortcutPath, requestedTarget, "打开 Meow Launcher"); err == nil {
		t.Fatal("expected a same-named shortcut for another executable to be preserved")
	}
	if target, err := readShortcutTarget(shortcutPath); err != nil || !strings.EqualFold(filepath.Clean(target), existingTarget) {
		t.Fatalf("existing shortcut changed: target=%q error=%v", target, err)
	}
}
