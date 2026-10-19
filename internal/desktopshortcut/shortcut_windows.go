//go:build windows

package desktopshortcut

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

const testDesktopRootEnvironment = "MEOW_DESKTOP_SHORTCUT_TEST_ROOT"

type Request struct {
	TargetExecutable string
	Description      string
}

func Ensure(request Request) (string, error) {
	target := filepath.Clean(strings.TrimSpace(request.TargetExecutable))
	if !filepath.IsAbs(target) {
		return "", errors.New("桌面快捷方式目标必须是绝对路径")
	}
	info, err := os.Lstat(target)
	if err != nil {
		return "", fmt.Errorf("桌面快捷方式目标不是已安装的普通文件: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("桌面快捷方式目标不是已安装的普通文件")
	}
	desktop, err := desktopDirectory()
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	if name == "" || name == "." {
		return "", errors.New("无法从应用入口生成快捷方式名称")
	}
	shortcutPath := filepath.Join(desktop, name+".lnk")
	if err := ensureAt(shortcutPath, target, strings.TrimSpace(request.Description)); err != nil {
		return "", err
	}
	return shortcutPath, nil
}

func RemoveIfTarget(targetExecutable string) error {
	target := filepath.Clean(strings.TrimSpace(targetExecutable))
	if !filepath.IsAbs(target) {
		return errors.New("快捷方式清理目标必须是绝对路径")
	}
	desktop, err := desktopDirectory()
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	shortcutPath := filepath.Join(desktop, name+".lnk")
	info, err := os.Lstat(shortcutPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("已有桌面快捷方式不是可安全清理的普通文件")
	}
	currentTarget, err := readShortcutTarget(shortcutPath)
	if err != nil {
		return fmt.Errorf("无法核验桌面快捷方式目标: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(currentTarget), target) {
		return nil
	}
	return os.Remove(shortcutPath)
}

func desktopDirectory() (string, error) {
	if override := strings.TrimSpace(os.Getenv(testDesktopRootEnvironment)); override != "" {
		machineRoot := strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_MACHINE_ROOT"))
		if !filepath.IsAbs(override) || !filepath.IsAbs(machineRoot) || !pathWithin(override, machineRoot) {
			return "", errors.New("测试桌面目录必须位于绝对的 MEOW_LAUNCHER_MACHINE_ROOT 内")
		}
		if err := os.MkdirAll(override, 0o700); err != nil {
			return "", err
		}
		return filepath.Clean(override), nil
	}
	desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", fmt.Errorf("无法定位当前用户桌面: %w", err)
	}
	if !filepath.IsAbs(desktop) {
		return "", errors.New("当前用户桌面不是绝对路径")
	}
	return filepath.Clean(desktop), nil
}

func ensureAt(shortcutPath, target, description string) error {
	if info, err := os.Lstat(shortcutPath); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("同名桌面入口不是普通快捷方式，未覆盖")
		}
		existingTarget, readErr := readShortcutTarget(shortcutPath)
		if readErr != nil {
			return fmt.Errorf("无法核验同名桌面快捷方式，未覆盖: %w", readErr)
		}
		if !strings.EqualFold(filepath.Base(filepath.Clean(existingTarget)), filepath.Base(target)) {
			return errors.New("桌面已有同名但属于其他程序的快捷方式，未覆盖")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(shortcutPath), ".meow-shortcut-*.lnk")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		return err
	}
	_ = os.Remove(temporaryPath)
	defer os.Remove(temporaryPath)
	if err := writeShortcut(temporaryPath, target, filepath.Dir(target), target+",0", description); err != nil {
		return fmt.Errorf("创建桌面快捷方式失败: %w", err)
	}
	if info, err := os.Lstat(temporaryPath); err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("生成的桌面快捷方式无效")
	}

	backupPath := ""
	if _, err := os.Lstat(shortcutPath); err == nil {
		backup, createErr := os.CreateTemp(filepath.Dir(shortcutPath), ".meow-shortcut-backup-*.lnk")
		if createErr != nil {
			return createErr
		}
		backupPath = backup.Name()
		if closeErr := backup.Close(); closeErr != nil {
			return closeErr
		}
		_ = os.Remove(backupPath)
		if renameErr := os.Rename(shortcutPath, backupPath); renameErr != nil {
			return renameErr
		}
	}
	if err := os.Rename(temporaryPath, shortcutPath); err != nil {
		if backupPath != "" {
			_ = os.Rename(backupPath, shortcutPath)
		}
		return err
	}
	if backupPath != "" {
		_ = os.Remove(backupPath)
	}
	return nil
}

func writeShortcut(shortcutPath, target, workingDirectory, iconLocation, description string) error {
	return withShortcut(shortcutPath, func(shortcut *ole.IDispatch) error {
		for name, value := range map[string]string{
			"TargetPath":       target,
			"WorkingDirectory": workingDirectory,
			"IconLocation":     iconLocation,
			"Description":      description,
		} {
			result, err := oleutil.PutProperty(shortcut, name, value)
			if result != nil {
				result.Clear()
			}
			if err != nil {
				return err
			}
		}
		result, err := oleutil.CallMethod(shortcut, "Save")
		if result != nil {
			result.Clear()
		}
		return err
	})
}

func readShortcutTarget(shortcutPath string) (string, error) {
	var target string
	err := withShortcut(shortcutPath, func(shortcut *ole.IDispatch) error {
		result, err := oleutil.GetProperty(shortcut, "TargetPath")
		if err != nil {
			return err
		}
		defer result.Clear()
		target = result.ToString()
		return nil
	})
	return target, err
}

func withShortcut(shortcutPath string, use func(*ole.IDispatch) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initialized := false
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		var oleErr *ole.OleError
		if !errors.As(err, &oleErr) || oleErr.Code() != 1 {
			return err
		}
	}
	initialized = true
	if initialized {
		defer ole.CoUninitialize()
	}
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()
	result, err := oleutil.CallMethod(shell, "CreateShortcut", shortcutPath)
	if err != nil {
		return err
	}
	shortcut := result.ToIDispatch()
	if shortcut == nil {
		result.Clear()
		return errors.New("WScript.Shell 未返回快捷方式对象")
	}
	defer shortcut.Release()
	return use(shortcut)
}

func pathWithin(candidate, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
