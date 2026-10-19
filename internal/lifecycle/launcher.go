package lifecycle

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"meow-launcher/internal/launchlocation"
)

const launcherPreviewFilename = "Meow Launcher v0.6.6 WIP.exe"

func OpenLauncherUpdate(productID, returnAction string) error {
	path, err := LocateLauncher()
	if err != nil {
		return err
	}
	args := []string{"--update-product", strings.TrimSpace(productID)}
	if strings.EqualFold(strings.TrimSpace(returnAction), "restart") {
		args = append(args, "--return", "restart")
	}
	command := exec.Command(path, args...)
	command.Dir = filepath.Dir(path)
	if err := command.Start(); err != nil {
		return errors.New("LAUNCHER_START_FAILED：无法启动 Meow Launcher")
	}
	return nil
}

func LocateLauncher() (string, error) {
	if override := strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_PATH")); override != "" {
		return requireExecutable(override)
	}
	if record, err := launchlocation.Load(); err == nil {
		return record.ExecutablePath, nil
	}
	candidates := []string{}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(dir, launcherPreviewFilename), filepath.Join(dir, "Meow Launcher.exe"))
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(workingDirectory, "dist", launcherPreviewFilename),
			filepath.Join(workingDirectory, "build", "bin", launcherPreviewFilename),
		)
	}
	for _, candidate := range candidates {
		if resolved, err := requireExecutable(candidate); err == nil {
			return resolved, nil
		}
	}
	return "", errors.New("LAUNCHER_NOT_FOUND：未找到 Meow Launcher；请先安装或修复 Launcher")
}

func requireExecutable(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return "", errors.New("Launcher 路径必须是绝对路径")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("Launcher 可执行文件不存在")
	}
	return path, nil
}
