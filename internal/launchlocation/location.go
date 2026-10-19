package launchlocation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"meow-launcher/internal/settings"
)

const SchemaVersion = "meow-launcher-location/v1"

type Record struct {
	SchemaVersion  string `json:"schema_version"`
	ExecutablePath string `json:"executable_path"`
	UpdatedAt      string `json:"updated_at"`
}

func Path() (string, error) {
	root, err := settings.MachineRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "runtime", "launcher-location.json"), nil
}

func Save(executablePath string) error {
	path, err := Path()
	if err != nil {
		return err
	}
	executablePath, err = requireExecutable(executablePath)
	if err != nil {
		return err
	}
	record := Record{SchemaVersion: SchemaVersion, ExecutablePath: executablePath, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}

func Load() (Record, error) {
	path, err := Path()
	if err != nil {
		return Record{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Record{}, errors.New("Launcher 位置登记不是普通文件")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil || record.SchemaVersion != SchemaVersion {
		return Record{}, errors.New("Launcher 位置登记无效")
	}
	record.ExecutablePath, err = requireExecutable(record.ExecutablePath)
	if err != nil {
		return Record{}, err
	}
	if _, err := time.Parse(time.RFC3339Nano, record.UpdatedAt); err != nil {
		return Record{}, errors.New("Launcher 位置登记时间无效")
	}
	return record, nil
}

func requireExecutable(value string) (string, error) {
	value = filepath.Clean(strings.TrimSpace(value))
	if value == "." || !filepath.IsAbs(value) || !strings.EqualFold(filepath.Ext(value), ".exe") {
		return "", errors.New("Launcher 位置必须是绝对 EXE 路径")
	}
	info, err := os.Lstat(value)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("Launcher 登记的可执行文件不存在")
	}
	return value, nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".launcher-location-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	backup := temporaryPath + ".previous"
	hadExisting := false
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("已有 Launcher 位置登记不是普通文件")
		}
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		hadExisting = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if hadExisting {
			_ = os.Rename(backup, path)
		}
		return err
	}
	if hadExisting {
		_ = os.Remove(backup)
	}
	return nil
}
