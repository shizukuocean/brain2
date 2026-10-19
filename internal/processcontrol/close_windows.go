//go:build windows

package processcontrol

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const wmClose = 0x0010

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	enumWindows              = user32.NewProc("EnumWindows")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	postMessage              = user32.NewProc("PostMessageW")
)

func closeExecutable(ctx context.Context, executablePath string) error {
	pids, err := executablePIDs(executablePath)
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return nil
	}

	requestClose(pids)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		remaining, scanErr := executablePIDs(executablePath)
		if scanErr != nil {
			return scanErr
		}
		if len(remaining) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("应用仍在运行；请保存工作并手动关闭后重试: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func executablePIDs(executablePath string) (map[uint32]struct{}, error) {
	target := normalizePath(executablePath)
	targetName := strings.ToLower(filepath.Base(target))
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("无法读取运行中的应用: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	result := map[uint32]struct{}{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err = windows.Process32First(snapshot, &entry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return result, nil
		}
		return nil, fmt.Errorf("无法读取运行中的应用: %w", err)
	}
	for {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if entry.ProcessID != uint32(os.Getpid()) && name == targetName {
			processPath, pathErr := queryProcessPath(entry.ProcessID)
			if pathErr != nil {
				return nil, fmt.Errorf("无法确认目标应用是否已经退出: %w", pathErr)
			}
			if normalizePath(processPath) == target {
				result[entry.ProcessID] = struct{}{}
			}
		}
		if err = windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, fmt.Errorf("无法继续读取运行中的应用: %w", err)
		}
	}
	return result, nil
}

func queryProcessPath(pid uint32) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

func requestClose(pids map[uint32]struct{}) {
	callback := windows.NewCallback(func(window uintptr, _ uintptr) uintptr {
		var pid uint32
		getWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&pid)))
		if _, ok := pids[pid]; ok {
			postMessage.Call(window, wmClose, 0, 0)
		}
		return 1
	})
	enumWindows.Call(callback, 0)
}

func normalizePath(value string) string {
	value = filepath.Clean(value)
	value = strings.TrimPrefix(value, `\\?\`)
	return strings.ToLower(value)
}
