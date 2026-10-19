package settings

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsEmptyState(t *testing.T) {
	state, err := LoadFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if state.Brain2Root != "" {
		t.Fatalf("expected empty root, got %q", state.Brain2Root)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config", "state.json")
	root := filepath.Join(dir, "Brain2 中文")
	if err := SaveTo(path, root); err != nil {
		t.Fatalf("SaveTo returned error: %v", err)
	}
	state, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}
	if state.SchemaVersion != schemaVersion {
		t.Fatalf("unexpected schema version: %q", state.SchemaVersion)
	}
	if state.Brain2Root != filepath.Clean(root) {
		t.Fatalf("unexpected root: %q", state.Brain2Root)
	}
	if state.UpdatedAt == "" {
		t.Fatal("updated_at was not written")
	}
	if state.LocalDeviceID != "" || state.SelectedDeviceID != "" {
		t.Fatalf("SaveTo guessed a device before local detection: %#v", state)
	}
	state.IgnoredConnectorVersions = map[string]string{"codex": "0.3.0"}
	if err := SaveStateTo(path, state); err != nil {
		t.Fatal(err)
	}
	secondRoot := filepath.Join(dir, "Brain2 second")
	if err := SaveTo(path, secondRoot); err != nil {
		t.Fatalf("second SaveTo returned error: %v", err)
	}
	state, err = LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom after update returned error: %v", err)
	}
	if state.Brain2Root != filepath.Clean(secondRoot) {
		t.Fatalf("state was not updated: %q", state.Brain2Root)
	}
	if state.IgnoredConnectorVersions["codex"] != "0.3.0" {
		t.Fatalf("updating the root discarded connector preferences: %#v", state.IgnoredConnectorVersions)
	}
}

func TestSaveRejectsEmptyRoot(t *testing.T) {
	err := SaveTo(filepath.Join(t.TempDir(), "state.json"), "  ")
	if err == nil {
		t.Fatal("expected empty root to be rejected")
	}
}

func TestDataRootHonorsIsolatedProfileOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "产品测试")
	t.Setenv("MEOW_LAUNCHER_DATA_ROOT", override)
	got, err := DataRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(override) {
		t.Fatalf("got %q, want %q", got, filepath.Clean(override))
	}
}

func TestProductRegistryIsStableAcrossProfileOverrides(t *testing.T) {
	machine := filepath.Join(t.TempDir(), "machine")
	profile := filepath.Join(t.TempDir(), "profile")
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machine)
	t.Setenv("MEOW_LAUNCHER_DATA_ROOT", profile)

	registry, err := ProductRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(machine, "runtime", "brain2-registry.json")
	if registry != want {
		t.Fatalf("registry followed profile: got %q, want %q", registry, want)
	}
	runtimeRoot, err := RuntimeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if runtimeRoot != filepath.Join(machine, "runtime") {
		t.Fatalf("unexpected runtime root: %q", runtimeRoot)
	}
}

func TestMachineRootRejectsRelativeOverride(t *testing.T) {
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", "relative-machine")
	if _, err := MachineRoot(); err == nil {
		t.Fatal("expected relative machine root to fail")
	}
}

func TestSettingsCanPersistObsidianBeforeBrain2FirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	obsidian := filepath.Join(t.TempDir(), "Apps", "Obsidian.exe")
	if err := SaveStateTo(path, State{ObsidianPath: obsidian}); err != nil {
		t.Fatal(err)
	}
	state, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Brain2Root != "" || state.ObsidianPath != filepath.Clean(obsidian) {
		t.Fatalf("unexpected first-run settings: %#v", state)
	}
}

func TestSettingsCanPersistMeowSystemRootWithoutDiscardingBrain2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	brain2Root := filepath.Join(t.TempDir(), "Brain2")
	if err := SaveTo(path, brain2Root); err != nil {
		t.Fatal(err)
	}
	meowSystemRoot := filepath.Join(t.TempDir(), "Apps", "Meow System")
	if err := SaveMeowSystemRootTo(path, meowSystemRoot); err != nil {
		t.Fatal(err)
	}
	state, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.Brain2Root != filepath.Clean(brain2Root) || state.MeowSystemRoot != filepath.Clean(meowSystemRoot) {
		t.Fatalf("unexpected combined settings: %#v", state)
	}
}

func TestDataRootRejectsRelativeOverride(t *testing.T) {
	t.Setenv("MEOW_LAUNCHER_DATA_ROOT", "relative-profile")
	if _, err := DataRoot(); err == nil {
		t.Fatal("expected relative override to fail")
	}
}
