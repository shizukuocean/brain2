package productregistry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = "0.1"

type Product struct {
	ProductID      string   `json:"product_id"`
	InstanceID     string   `json:"instance_id"`
	Installed      bool     `json:"installed"`
	Status         string   `json:"status"`
	RootPath       string   `json:"root_path"`
	ConfigPath     string   `json:"config_path"`
	ProjectsPath   string   `json:"projects_path"`
	VersionPath    string   `json:"version_path"`
	Capabilities   []string `json:"capabilities"`
	LastVerifiedAt string   `json:"last_verified_at"`
}

type Registry struct {
	SchemaVersion string    `json:"schema_version"`
	Products      []Product `json:"products"`
}

func LoadFrom(path string) (Registry, error) {
	registry := Registry{SchemaVersion: SchemaVersion, Products: []Product{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return registry, nil
	}
	if err != nil {
		return Registry{}, err
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return Registry{}, err
	}
	if registry.Products == nil {
		registry.Products = []Product{}
	}
	return registry, nil
}

func UpsertBrain2(path, root, instanceID string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	instanceID = strings.TrimSpace(instanceID)
	if root == "." || root == "" || instanceID == "" {
		return errors.New("Brain2 产品登记缺少有效根目录或实例 ID")
	}
	configPath := filepath.Join(root, ".brain2", "config.json")
	projectsPath := filepath.Join(root, ".brain2", "projects.json")
	versionPath := filepath.Join(root, ".brain2", "version.json")
	for _, required := range []string{configPath, projectsPath, versionPath} {
		info, err := os.Stat(required)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("Brain2 产品登记前校验失败：缺少 " + required)
		}
	}
	registry, err := LoadFrom(path)
	if err != nil {
		return err
	}
	product := Product{
		ProductID: "brain2", InstanceID: instanceID, Installed: true, Status: "ready",
		RootPath: root, ConfigPath: configPath, ProjectsPath: projectsPath, VersionPath: versionPath,
		Capabilities:   []string{"project_registry", "project_context_provider", "path_provider"},
		LastVerifiedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	replaced := false
	for i := range registry.Products {
		if registry.Products[i].ProductID == product.ProductID && registry.Products[i].InstanceID == product.InstanceID {
			registry.Products[i] = product
			replaced = true
			break
		}
	}
	if !replaced {
		registry.Products = append(registry.Products, product)
	}
	registry.SchemaVersion = SchemaVersion
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0o600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".meow-product-registry-*.tmp")
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
