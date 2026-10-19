package brain2

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const deviceRegistrySchemaVersion = "0.1"

type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Brain2Path  string `json:"brain2_path"`
	MachineName string `json:"machine_name,omitempty"`
	Status      string `json:"status"`
}

type DeviceRegistry struct {
	SchemaVersion string   `json:"schema_version"`
	Devices       []Device `json:"devices"`
}

type LocalDeviceResolution struct {
	DeviceID string
	Source   string
}

func NormalizeDeviceID(deviceID string) (string, error) {
	deviceID = strings.ToUpper(strings.TrimSpace(deviceID))
	if deviceID != "A" && deviceID != "B" {
		return "", errors.New("当前版本的设备代码只能是 A 或 B")
	}
	return deviceID, nil
}

func LoadDevices(root string) (DeviceRegistry, error) {
	var registry DeviceRegistry
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "devices.json"))
	if err != nil {
		return registry, err
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return registry, err
	}
	if registry.Devices == nil {
		registry.Devices = []Device{}
	}
	return registry, nil
}

func LoadDevicesOrEmpty(root string) (DeviceRegistry, error) {
	registry, err := LoadDevices(root)
	if os.IsNotExist(err) {
		return DeviceRegistry{SchemaVersion: deviceRegistrySchemaVersion, Devices: []Device{}}, nil
	}
	return registry, err
}

func DeviceByID(registry DeviceRegistry, deviceID string) (Device, bool) {
	deviceID = strings.ToUpper(strings.TrimSpace(deviceID))
	for _, device := range registry.Devices {
		if device.ID == deviceID {
			return device, true
		}
	}
	return Device{}, false
}

func InferLegacyDeviceID(brain2Root, fallback string) string {
	value := strings.ToUpper(filepath.Clean(strings.TrimSpace(brain2Root)))
	if strings.HasPrefix(value, `D:\`) {
		return "B"
	}
	if strings.HasPrefix(value, `E:\`) {
		return "A"
	}
	if normalized, err := NormalizeDeviceID(fallback); err == nil {
		return normalized
	}
	return "A"
}

func ResolveLocalDeviceID(registry DeviceRegistry, root, hostname, persistedDeviceID string) LocalDeviceResolution {
	hostname = strings.TrimSpace(hostname)
	persistedDeviceID, _ = NormalizeDeviceID(persistedDeviceID)

	automatic := detectLocalDeviceID(registry, root, hostname)
	if persistedDeviceID == "" {
		return automatic
	}
	if device, exists := DeviceByID(registry, persistedDeviceID); exists {
		if hostname != "" && strings.EqualFold(strings.TrimSpace(device.MachineName), hostname) {
			return LocalDeviceResolution{DeviceID: persistedDeviceID, Source: "local_setting+machine_name"}
		}
		if sameWorkspaceRoot(device.Brain2Path, root) {
			return LocalDeviceResolution{DeviceID: persistedDeviceID, Source: "local_setting+brain2_path"}
		}
		if automatic.DeviceID != "" && automatic.DeviceID != persistedDeviceID {
			return automatic
		}
		if automatic.DeviceID == "" {
			return LocalDeviceResolution{}
		}
		return LocalDeviceResolution{DeviceID: persistedDeviceID, Source: "local_setting"}
	}
	if automatic.DeviceID != "" {
		return automatic
	}
	return LocalDeviceResolution{DeviceID: persistedDeviceID, Source: "local_setting"}
}

func detectLocalDeviceID(registry DeviceRegistry, root, hostname string) LocalDeviceResolution {
	if hostname != "" {
		matches := []string{}
		for _, device := range registry.Devices {
			if strings.EqualFold(strings.TrimSpace(device.MachineName), hostname) {
				matches = append(matches, device.ID)
			}
		}
		if len(matches) == 1 {
			return LocalDeviceResolution{DeviceID: matches[0], Source: "machine_name"}
		}
	}
	pathMatches := []string{}
	for _, device := range registry.Devices {
		if sameWorkspaceRoot(device.Brain2Path, root) {
			pathMatches = append(pathMatches, device.ID)
		}
	}
	if len(pathMatches) == 1 {
		return LocalDeviceResolution{DeviceID: pathMatches[0], Source: "brain2_path"}
	}

	if len(registry.Devices) == 0 {
		pathDeviceID := InferLegacyDeviceID(root, "")
		if strings.HasPrefix(strings.ToUpper(filepath.Clean(strings.TrimSpace(root))), `D:\`) {
			return LocalDeviceResolution{DeviceID: pathDeviceID, Source: "brain2_path_convention"}
		}
		if strings.HasPrefix(strings.ToUpper(filepath.Clean(strings.TrimSpace(root))), `E:\`) {
			return LocalDeviceResolution{DeviceID: pathDeviceID, Source: "brain2_path_convention"}
		}
		return LocalDeviceResolution{DeviceID: "A", Source: "first_device"}
	}
	return LocalDeviceResolution{}
}

func sameWorkspaceRoot(left, right string) bool {
	leftIdentity, leftOK := workspaceRootIdentity(left)
	rightIdentity, rightOK := workspaceRootIdentity(right)
	return leftOK && rightOK && leftIdentity == rightIdentity
}

func EnsureDeviceRegistry(root, localDeviceID string) (DeviceRegistry, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return DeviceRegistry{}, err
	}
	localDeviceID, err = NormalizeDeviceID(localDeviceID)
	if err != nil {
		return DeviceRegistry{}, err
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		return DeviceRegistry{}, err
	}
	registry, err := LoadDevices(root)
	registryCreated := false
	if os.IsNotExist(err) {
		registry = DeviceRegistry{SchemaVersion: deviceRegistrySchemaVersion, Devices: []Device{}}
		registryCreated = true
	} else if err != nil {
		return DeviceRegistry{}, err
	}
	original := DeviceRegistry{SchemaVersion: registry.SchemaVersion, Devices: append([]Device(nil), registry.Devices...)}
	registry.SchemaVersion = deviceRegistrySchemaVersion
	for index := range registry.Devices {
		registry.Devices[index].Status = "registered"
	}
	hostname, _ := os.Hostname()
	registry = upsertDevice(registry, Device{ID: localDeviceID, Name: "设备 " + localDeviceID, Brain2Path: root, MachineName: hostname, Status: "registered"})
	legacyDeviceID := InferLegacyDeviceID(cfg.Brain2Root, localDeviceID)
	if registryCreated && legacyDeviceID != localDeviceID {
		registry = upsertDeviceIfMissing(registry, Device{ID: legacyDeviceID, Name: "设备 " + legacyDeviceID, Brain2Path: filepath.Clean(cfg.Brain2Root), MachineName: cfg.DeviceName, Status: "registered"})
	}
	if registryCreated || !reflect.DeepEqual(original, registry) {
		if err := writeJSON(filepath.Join(root, ".brain2", "devices.json"), registry); err != nil {
			return DeviceRegistry{}, err
		}
	}
	if err := migrateLegacyAgents(root, legacyDeviceID); err != nil {
		return DeviceRegistry{}, err
	}
	return registry, nil
}

func AddDevice(root, deviceID, localDeviceID string) (DeviceRegistry, error) {
	registry, err := EnsureDeviceRegistry(root, localDeviceID)
	if err != nil {
		return DeviceRegistry{}, err
	}
	deviceID, err = NormalizeDeviceID(deviceID)
	if err != nil {
		return DeviceRegistry{}, err
	}
	if _, exists := DeviceByID(registry, deviceID); exists {
		return registry, nil
	}
	path := `D:\Brain2`
	if deviceID == "A" {
		path = `E:\Brain2`
	}
	registry = upsertDevice(registry, Device{ID: deviceID, Name: "设备 " + deviceID, Brain2Path: path, Status: "registered"})
	if err := writeJSON(filepath.Join(root, ".brain2", "devices.json"), registry); err != nil {
		return DeviceRegistry{}, err
	}
	return registry, nil
}

func RemoveDevice(root, deviceID, localDeviceID string) (DeviceRegistry, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return DeviceRegistry{}, err
	}
	deviceID, err = NormalizeDeviceID(deviceID)
	if err != nil {
		return DeviceRegistry{}, err
	}
	localDeviceID, err = NormalizeDeviceID(localDeviceID)
	if err != nil {
		return DeviceRegistry{}, err
	}
	if deviceID == localDeviceID {
		return DeviceRegistry{}, errors.New("不能移除当前本机设备；请先在另一台机器完成本机登记")
	}
	registry, err := LoadDevices(root)
	if err != nil {
		return DeviceRegistry{}, err
	}
	removed := false
	devices := make([]Device, 0, len(registry.Devices))
	for _, device := range registry.Devices {
		if device.ID == deviceID {
			removed = true
			continue
		}
		devices = append(devices, device)
	}
	if !removed {
		return DeviceRegistry{}, errors.New("该设备不在当前管理列表中")
	}
	registry.Devices = devices
	if err := writeJSON(filepath.Join(root, ".brain2", "devices.json"), registry); err != nil {
		return DeviceRegistry{}, err
	}
	return registry, nil
}

func LoadAgentsForDevice(root, deviceID string) (AgentRegistry, error) {
	deviceID, err := NormalizeDeviceID(deviceID)
	if err != nil {
		return AgentRegistry{}, err
	}
	registry, err := LoadAgents(root)
	if err != nil {
		return AgentRegistry{}, err
	}
	filtered := AgentRegistry{SchemaVersion: registry.SchemaVersion, Agents: []Agent{}}
	for _, agent := range registry.Agents {
		if agent.DeviceID == deviceID {
			filtered.Agents = append(filtered.Agents, agent)
		}
	}
	return filtered, nil
}

func migrateLegacyAgents(root, legacyDeviceID string) error {
	registry, err := LoadAgents(root)
	if err != nil {
		return err
	}
	changed := false
	for index := range registry.Agents {
		if strings.TrimSpace(registry.Agents[index].DeviceID) == "" {
			registry.Agents[index].DeviceID = legacyDeviceID
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeJSON(filepath.Join(root, ".brain2", "agents.json"), registry)
}

func upsertDevice(registry DeviceRegistry, device Device) DeviceRegistry {
	for index := range registry.Devices {
		if registry.Devices[index].ID == device.ID {
			registry.Devices[index] = device
			return registry
		}
	}
	registry.Devices = append(registry.Devices, device)
	return registry
}

func upsertDeviceIfMissing(registry DeviceRegistry, device Device) DeviceRegistry {
	if _, exists := DeviceByID(registry, device.ID); exists {
		return registry
	}
	registry.Devices = append(registry.Devices, device)
	return registry
}
