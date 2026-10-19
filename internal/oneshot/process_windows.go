//go:build windows

package oneshot

import "golang.org/x/sys/windows"

func processRunning(pid int) (bool, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return false, nil
		}
		return false, err
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, err
	}
	return status == uint32(windows.WAIT_TIMEOUT), nil
}
