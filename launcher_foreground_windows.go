//go:build launcher && windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	launcherUser32                   = syscall.NewLazyDLL("user32.dll")
	launcherEnumWindows              = launcherUser32.NewProc("EnumWindows")
	launcherGetWindowThreadProcessID = launcherUser32.NewProc("GetWindowThreadProcessId")
	launcherGetClassNameW            = launcherUser32.NewProc("GetClassNameW")
	launcherShowWindow               = launcherUser32.NewProc("ShowWindow")
	launcherSetForegroundWindow      = launcherUser32.NewProc("SetForegroundWindow")
	launcherBringWindowToTop         = launcherUser32.NewProc("BringWindowToTop")
	launcherSetWindowPos             = launcherUser32.NewProc("SetWindowPos")
)

const (
	launcherSWRestore     = 9
	launcherSWPNoSize     = 0x0001
	launcherSWPNoMove     = 0x0002
	launcherSWPShowWindow = 0x0040
	launcherHWNDTopmost   = ^uintptr(0) // HWND_TOPMOST = -1
	launcherHWNDNoTopmost = ^uintptr(1) // HWND_NOTOPMOST = -2
)

// bringLauncherWindowToFront finds the Wails top-level window owned by this
// process. The temporary topmost toggle is immediately removed; it is only used
// to make a user-requested cross-application hand-off visible and does not make
// Launcher a permanently always-on-top application.
func bringLauncherWindowToFront() {
	pid := uint32(os.Getpid())
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var windowPID uint32
		launcherGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID != pid || launcherWindowClass(hwnd) != "MeowLauncherLifecycleWindow" {
			return 1
		}

		launcherShowWindow.Call(hwnd, launcherSWRestore)
		launcherSetWindowPos.Call(hwnd, launcherHWNDTopmost, 0, 0, 0, 0, launcherSWPNoMove|launcherSWPNoSize|launcherSWPShowWindow)
		launcherSetWindowPos.Call(hwnd, launcherHWNDNoTopmost, 0, 0, 0, 0, launcherSWPNoMove|launcherSWPNoSize|launcherSWPShowWindow)
		launcherBringWindowToTop.Call(hwnd)
		launcherSetForegroundWindow.Call(hwnd)
		return 0
	})
	launcherEnumWindows.Call(callback, 0)
}

func launcherWindowClass(hwnd uintptr) string {
	buffer := make([]uint16, 256)
	length, _, _ := launcherGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:length])
}
