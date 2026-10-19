package brain2

import (
	"path/filepath"
	"testing"
)

func TestLegacySyncedDeviceStateMigratesToBWithoutHidingA(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Brain2Root = `D:\Brain2`
	cfg.DeviceName = "EXAMPLE-HOST-1"
	if err := writeJSON(filepath.Join(root, ".brain2", "config.json"), cfg); err != nil {
		t.Fatal(err)
	}
	legacy := AgentRegistry{SchemaVersion: schemaVersion, Agents: []Agent{{ID: "codex", Name: "Codex CLI", ConnectionStatus: string(ConnectionConnected)}}}
	if err := writeJSON(filepath.Join(root, ".brain2", "agents.json"), legacy); err != nil {
		t.Fatal(err)
	}

	registry, err := EnsureDeviceRegistry(root, "A")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := DeviceByID(registry, "A"); !ok {
		t.Fatal("local device A was not registered")
	}
	if device, ok := DeviceByID(registry, "B"); !ok || device.Brain2Path != `D:\Brain2` {
		t.Fatalf("legacy device B was not preserved: %#v", registry.Devices)
	}
	aAgents, err := LoadAgentsForDevice(root, "A")
	if err != nil {
		t.Fatal(err)
	}
	bAgents, err := LoadAgentsForDevice(root, "B")
	if err != nil {
		t.Fatal(err)
	}
	if len(aAgents.Agents) != 0 || len(bAgents.Agents) != 1 || bAgents.Agents[0].DeviceID != "B" {
		t.Fatalf("legacy agents were not isolated by device: A=%#v B=%#v", aAgents, bAgents)
	}
	if !IsWorkspace(root) {
		t.Fatal("a synchronized logical Brain2 was rejected because its legacy path came from B")
	}
}

func TestSameAgentIDCanCoexistOnAAndB(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDeviceRegistry(root, "A"); err != nil {
		t.Fatal(err)
	}
	if err := updateAgentForDevice(root, "A", "codex", func(agent *Agent) { agent.ConnectionStatus = string(ConnectionRegistered) }); err != nil {
		t.Fatal(err)
	}
	if err := updateAgentForDevice(root, "B", "codex", func(agent *Agent) { agent.ConnectionStatus = string(ConnectionConnected) }); err != nil {
		t.Fatal(err)
	}
	aAgents, _ := LoadAgentsForDevice(root, "A")
	bAgents, _ := LoadAgentsForDevice(root, "B")
	if len(aAgents.Agents) != 1 || aAgents.Agents[0].ConnectionStatus != string(ConnectionRegistered) {
		t.Fatalf("unexpected A record: %#v", aAgents)
	}
	if len(bAgents.Agents) != 1 || bAgents.Agents[0].ConnectionStatus != string(ConnectionConnected) {
		t.Fatalf("unexpected B record: %#v", bAgents)
	}
}

func TestLocalDeviceResolutionCorrectsLegacyDefaultAOnMachineB(t *testing.T) {
	registry := DeviceRegistry{SchemaVersion: deviceRegistrySchemaVersion, Devices: []Device{
		{ID: "A", Brain2Path: `E:\Brain2`, MachineName: "WINDFLOWER"},
		{ID: "B", Brain2Path: `D:\Brain2`, MachineName: "EXAMPLE-HOST-1"},
	}}
	resolution := ResolveLocalDeviceID(registry, `D:\Brain2`, "EXAMPLE-HOST-1", "A")
	if resolution.DeviceID != "B" || resolution.Source != "machine_name" {
		t.Fatalf("legacy A default was not corrected on B: %#v", resolution)
	}
}

func TestLocalDeviceResolutionRequiresChoiceWhenEvidenceIsAmbiguous(t *testing.T) {
	registry := DeviceRegistry{SchemaVersion: deviceRegistrySchemaVersion, Devices: []Device{
		{ID: "A", Brain2Path: `C:\Other\Brain2`, MachineName: "OTHER-PC"},
	}}
	resolution := ResolveLocalDeviceID(registry, filepath.Join(t.TempDir(), "Brain2"), "UNKNOWN-PC", "")
	if resolution.DeviceID != "" {
		t.Fatalf("ambiguous machine was guessed as %#v", resolution)
	}
}

func TestConflictingLegacyLocalSettingRequiresChoice(t *testing.T) {
	registry := DeviceRegistry{SchemaVersion: deviceRegistrySchemaVersion, Devices: []Device{
		{ID: "A", Brain2Path: `E:\Brain2`, MachineName: "WINDFLOWER"},
	}}
	resolution := ResolveLocalDeviceID(registry, `D:\Brain2`, "UNKNOWN-B-PC", "A")
	if resolution.DeviceID != "" {
		t.Fatalf("conflicting legacy A setting was trusted without evidence: %#v", resolution)
	}
}

func TestRemovingRemoteDevicePreservesHistoryForReAdd(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDeviceRegistry(root, "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddDevice(root, "B", "A"); err != nil {
		t.Fatal(err)
	}
	if err := updateAgentForDevice(root, "B", "codex", func(agent *Agent) { agent.ConnectionStatus = string(ConnectionConnected) }); err != nil {
		t.Fatal(err)
	}
	registry, err := RemoveDevice(root, "B", "A")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := DeviceByID(registry, "B"); exists {
		t.Fatal("B remained in the management list")
	}
	registry, err = EnsureDeviceRegistry(root, "A")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := DeviceByID(registry, "B"); exists {
		t.Fatal("removed B was silently re-added from legacy config")
	}
	history, err := LoadAgentsForDevice(root, "B")
	if err != nil || len(history.Agents) != 1 {
		t.Fatalf("B history was deleted: %#v err=%v", history, err)
	}
	if _, err := AddDevice(root, "B", "A"); err != nil {
		t.Fatal(err)
	}
	restored, _ := LoadAgentsForDevice(root, "B")
	if len(restored.Agents) != 1 || restored.Agents[0].ConnectionStatus != string(ConnectionConnected) {
		t.Fatalf("B history did not reappear after re-add: %#v", restored)
	}
	if _, err := RemoveDevice(root, "A", "A"); err == nil {
		t.Fatal("removing the local device should be rejected")
	}
}
