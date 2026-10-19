package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const schemaVersion = "0.1"

type State struct {
	SchemaVersion            string            `json:"schema_version"`
	Brain2Root               string            `json:"brain2_root"`
	MeowSystemRoot           string            `json:"meow_system_root,omitempty"`
	LocalDeviceID            string            `json:"local_device_id,omitempty"`
	SelectedDeviceID         string            `json:"selected_device_id,omitempty"`
	ObsidianPath             string            `json:"obsidian_path,omitempty"`
	IgnoredConnectorVersions map[string]string `json:"ignored_connector_versions,omitempty"`
	UpdatedAt                string            `json:"updated_at"`
}

func Path() (string, error) {
	root, err := DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "state", "state.json"), nil
}

func ProductRegistryPath() (string, error) {
	root, err := MachineRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runtime", "brain2-registry.json"), nil
}

// RuntimeRoot is stable across Launcher profiles. Agent MCP registrations must
// point here so switching workspaces or profiles never requires reinstalling
// the connector.
func RuntimeRoot() (string, error) {
	root, err := MachineRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runtime"), nil
}

// MachineRoot returns the per-user, per-device installation root. It never
// guesses a drive letter. Tests and managed deployments may override it with
// an explicit absolute path.
func MachineRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_MACHINE_ROOT")); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("Launcher 本机目录必须是绝对路径")
		}
		return filepath.Clean(override), nil
	}
	if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
		return filepath.Join(local, "MeowLauncher"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "MeowLauncher"), nil
}

// DataRoot stores profile-local state and WebView2 data. The optional override
// isolates test profiles without changing the stable machine registry.
func DataRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_DATA_ROOT")); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("Launcher 数据目录必须是绝对路径")
		}
		return filepath.Clean(override), nil
	}
	return MachineRoot()
}

func Load() (State, error) {
	path, err := Path()
	if err != nil {
		return State{}, err
	}
	return LoadFrom(path)
}

func LoadFrom(path string) (State, error) {
	var state State
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.IgnoredConnectorVersions == nil {
		state.IgnoredConnectorVersions = map[string]string{}
	}
	return state, nil
}

func Save(root string) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveTo(path, root)
}

func SaveTo(path, root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("Brain2 目录不能为空")
	}
	state, err := LoadFrom(path)
	if err != nil {
		return err
	}
	state.Brain2Root = root
	return SaveStateTo(path, state)
}

func SaveMeowSystemRoot(root string) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveMeowSystemRootTo(path, root)
}

func SaveMeowSystemRootTo(path, root string) error {
	root = strings.TrimSpace(root)
	if root == "" || !filepath.IsAbs(root) {
		return errors.New("Meow System 目录必须是绝对路径")
	}
	state, err := LoadFrom(path)
	if err != nil {
		return err
	}
	state.MeowSystemRoot = filepath.Clean(root)
	return SaveStateTo(path, state)
}

func SaveState(state State) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveStateTo(path, state)
}

func SaveStateTo(path string, state State) error {
	root := state.Brain2Root
	root = strings.TrimSpace(root)
	if root != "" {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		state.Brain2Root = filepath.Clean(absRoot)
	} else {
		state.Brain2Root = ""
	}
	state.SchemaVersion = schemaVersion
	state.MeowSystemRoot = strings.TrimSpace(state.MeowSystemRoot)
	if state.MeowSystemRoot != "" {
		if !filepath.IsAbs(state.MeowSystemRoot) {
			return errors.New("Meow System 目录必须是绝对路径")
		}
		state.MeowSystemRoot = filepath.Clean(state.MeowSystemRoot)
	}
	state.LocalDeviceID = strings.ToUpper(strings.TrimSpace(state.LocalDeviceID))
	state.SelectedDeviceID = strings.ToUpper(strings.TrimSpace(state.SelectedDeviceID))
	if state.LocalDeviceID != "" && state.LocalDeviceID != "A" && state.LocalDeviceID != "B" {
		return errors.New("本机设备代码只能是 A 或 B")
	}
	if state.SelectedDeviceID != "" && state.SelectedDeviceID != "A" && state.SelectedDeviceID != "B" {
		return errors.New("当前查看设备代码只能是 A 或 B")
	}
	state.ObsidianPath = filepath.Clean(strings.TrimSpace(state.ObsidianPath))
	if state.ObsidianPath == "." {
		state.ObsidianPath = ""
	}
	if state.IgnoredConnectorVersions == nil {
		state.IgnoredConnectorVersions = map[string]string{}
	}
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data, 0o600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".meow-launcher-state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.Rename(tmpPath, path)
	}
	backup := path + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
