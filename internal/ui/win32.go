//go:build windows

package ui

import (
	"syscall"
	"unsafe"
)

type HWND uintptr
type HINSTANCE uintptr
type HMENU uintptr
type HBRUSH uintptr
type HICON uintptr
type HCURSOR uintptr
type HGDIOBJ uintptr
type LPARAM uintptr
type WPARAM uintptr
type LRESULT uintptr

type POINT struct{ X, Y int32 }
type MSG struct {
	Hwnd     HWND
	Message  uint32
	WParam   WPARAM
	LParam   LPARAM
	Time     uint32
	Pt       POINT
	LPrivate uint32
}
type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     HINSTANCE
	HIcon         HICON
	HCursor       HCURSOR
	HbrBackground HBRUSH
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       HICON
}

type RECT struct{ Left, Top, Right, Bottom int32 }

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pRegisterClassExW     = user32.NewProc("RegisterClassExW")
	pCreateWindowExW      = user32.NewProc("CreateWindowExW")
	pDefWindowProcW       = user32.NewProc("DefWindowProcW")
	pShowWindow           = user32.NewProc("ShowWindow")
	pUpdateWindow         = user32.NewProc("UpdateWindow")
	pGetMessageW          = user32.NewProc("GetMessageW")
	pTranslateMessage     = user32.NewProc("TranslateMessage")
	pDispatchMessageW     = user32.NewProc("DispatchMessageW")
	pPostQuitMessage      = user32.NewProc("PostQuitMessage")
	pLoadCursorW          = user32.NewProc("LoadCursorW")
	pSetWindowTextW       = user32.NewProc("SetWindowTextW")
	pGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	pGetWindowTextW       = user32.NewProc("GetWindowTextW")
	pSendMessageW         = user32.NewProc("SendMessageW")
	pMessageBoxW          = user32.NewProc("MessageBoxW")
	pEnableWindow         = user32.NewProc("EnableWindow")
	pSetWindowPos         = user32.NewProc("SetWindowPos")
	pGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	pShellExecuteW        = shell32.NewProc("ShellExecuteW")
	pCoInitializeEx       = ole32.NewProc("CoInitializeEx")
	pCoUninitialize       = ole32.NewProc("CoUninitialize")
)

const (
	CS_HREDRAW               = 0x0002
	CS_VREDRAW               = 0x0001
	WS_OVERLAPPEDWINDOW      = 0x00CF0000
	WS_VISIBLE               = 0x10000000
	WS_CHILD                 = 0x40000000
	WS_TABSTOP               = 0x00010000
	WS_BORDER                = 0x00800000
	WS_VSCROLL               = 0x00200000
	BS_PUSHBUTTON            = 0x00000000
	BS_DEFPUSHBUTTON         = 0x00000001
	SS_LEFT                  = 0x00000000
	SS_CENTER                = 0x00000001
	CBS_DROPDOWNLIST         = 0x0003
	LBS_NOTIFY               = 0x0001
	SW_SHOW                  = 5
	SW_HIDE                  = 0
	WM_DESTROY               = 0x0002
	WM_COMMAND               = 0x0111
	WM_SETFONT               = 0x0030
	CB_ADDSTRING             = 0x0143
	CB_GETCURSEL             = 0x0147
	CB_SETCURSEL             = 0x014E
	LB_ADDSTRING             = 0x0180
	LB_RESETCONTENT          = 0x0184
	MB_OK                    = 0x00000000
	MB_ICONINFORMATION       = 0x00000040
	MB_ICONWARNING           = 0x00000030
	MB_ICONERROR             = 0x00000010
	IDC_ARROW                = 32512
	COLOR_WINDOW             = 5
	SWP_NOSIZE               = 0x0001
	SWP_NOMOVE               = 0x0002
	SWP_NOZORDER             = 0x0004
	SWP_SHOWWINDOW           = 0x0040
	SWP_HIDEWINDOW           = 0x0080
	COINIT_APARTMENTTHREADED = 0x2
)

func utf16Ptr(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func loword(v uintptr) uint16   { return uint16(v & 0xffff) }
func hiword(v uintptr) uint16   { return uint16((v >> 16) & 0xffff) }

func createWindow(exStyle uint32, class, text string, style uint32, x, y, w, h int32, parent HWND, id int32, inst HINSTANCE) HWND {
	r, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(utf16Ptr(class))), uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(parent), uintptr(id), uintptr(inst), 0)
	return HWND(r)
}

func setText(hwnd HWND, text string) {
	pSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(utf16Ptr(text))))
}
func getText(hwnd HWND) string {
	n, _, _ := pGetWindowTextLengthW.Call(uintptr(hwnd))
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}
func show(hwnd HWND, visible bool) {
	flags := uintptr(SWP_NOSIZE | SWP_NOMOVE | SWP_NOZORDER)
	if visible {
		flags |= SWP_SHOWWINDOW
	} else {
		flags |= SWP_HIDEWINDOW
	}
	pSetWindowPos.Call(uintptr(hwnd), 0, 0, 0, 0, 0, flags)
}
func enable(hwnd HWND, enabled bool) {
	v := uintptr(0)
	if enabled {
		v = 1
	}
	pEnableWindow.Call(uintptr(hwnd), v)
}
func msgBox(parent HWND, text, title string, flags uint32) {
	pMessageBoxW.Call(uintptr(parent), uintptr(unsafe.Pointer(utf16Ptr(text))), uintptr(unsafe.Pointer(utf16Ptr(title))), uintptr(flags))
}
