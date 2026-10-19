//go:build windows

package installerui

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	shell32              = syscall.NewLazyDLL("shell32.dll")
	ole32                = syscall.NewLazyDLL("ole32.dll")
	messageBoxW          = user32.NewProc("MessageBoxW")
	shBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	shGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	coTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	coInitializeEx       = ole32.NewProc("CoInitializeEx")
	coUninitialize       = ole32.NewProc("CoUninitialize")
)

const (
	mbOK              = 0x00000000
	mbYesNo           = 0x00000004
	mbIconInformation = 0x00000040
	mbIconWarning     = 0x00000030
	mbIconError       = 0x00000010
	idYes             = 6
	coinitApartment   = 0x2
	bifReturnOnlyFS   = 0x00000001
	bifNewDialogStyle = 0x00000040
)

type browseInfo struct {
	owner       uintptr
	root        uintptr
	displayName *uint16
	title       *uint16
	flags       uint32
	callback    uintptr
	lParam      uintptr
	image       int32
}

func ChooseFolder(title string) (string, bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initialized, _, _ := coInitializeEx.Call(0, coinitApartment)
	if initialized == 0 || initialized == 1 {
		defer coUninitialize.Call()
	}
	displayName := make([]uint16, 260)
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return "", false, err
	}
	info := browseInfo{
		displayName: &displayName[0], title: titlePtr,
		flags: bifReturnOnlyFS | bifNewDialogStyle,
	}
	pidl, _, callErr := shBrowseForFolderW.Call(uintptr(unsafe.Pointer(&info)))
	if pidl == 0 {
		if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
			return "", false, callErr
		}
		return "", false, nil
	}
	defer coTaskMemFree.Call(pidl)
	pathBuffer := make([]uint16, 32768)
	ok, _, callErr := shGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&pathBuffer[0])))
	if ok == 0 {
		if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
			return "", false, callErr
		}
		return "", false, errors.New("Windows 未能返回所选目录")
	}
	return syscall.UTF16ToString(pathBuffer), true, nil
}

func ChooseFolderFrom(title, _ string) (string, bool, error) {
	return ChooseFolder(title)
}

func Confirm(text, title string) bool {
	return messageBox(text, title, mbYesNo|mbIconWarning) == idYes
}

func ShowError(text, title string) {
	messageBox(text, title, mbOK|mbIconError)
}

func ShowInfo(text, title string) {
	messageBox(text, title, mbOK|mbIconInformation)
}

func messageBox(text, title string, flags uint32) uintptr {
	textPtr, _ := syscall.UTF16PtrFromString(text)
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	result, _, _ := messageBoxW.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), uintptr(flags))
	return result
}
