//go:build windows

package processcontrol

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const helperWindowEnvironment = "MEOW_PROCESS_CONTROL_WINDOW_HELPER"

func TestCloseExecutableClosesExactRunningGUIProcess(t *testing.T) {
	readyPath := filepath.Join(t.TempDir(), "window-ready")
	command := exec.Command(os.Args[0], "-test.run=TestProcessControlWindowHelper")
	command.Env = append(os.Environ(), helperWindowEnvironment+"=1", "MEOW_PROCESS_CONTROL_READY="+readyPath)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("hidden helper window did not become ready")
		}
		time.Sleep(25 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := CloseExecutable(ctx, os.Args[0]); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("helper process did not exit cleanly: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("helper process remained after its hidden window received WM_CLOSE")
	}
}

func TestProcessControlWindowHelper(t *testing.T) {
	if os.Getenv(helperWindowEnvironment) != "1" {
		t.Skip("helper process only")
	}
	if err := runHiddenHelperWindow(os.Getenv("MEOW_PROCESS_CONTROL_READY")); err != nil {
		t.Fatal(err)
	}
}

type windowClassEx struct {
	Size        uint32
	Style       uint32
	WindowProc  uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    uintptr
	Icon        uintptr
	Cursor      uintptr
	Background  uintptr
	MenuName    *uint16
	ClassName   *uint16
	IconSmall   uintptr
}

type windowMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PointX  int32
	PointY  int32
	Private uint32
}

func runHiddenHelperWindow(readyPath string) error {
	windowClassName, _ := windows.UTF16PtrFromString("MeowProcessControlIntegrationWindow")
	windowTitle, _ := windows.UTF16PtrFromString("Meow process control integration")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getModuleHandle := kernel32.NewProc("GetModuleHandleW")
	registerClass := user32.NewProc("RegisterClassExW")
	createWindow := user32.NewProc("CreateWindowExW")
	destroyWindow := user32.NewProc("DestroyWindow")
	defWindowProc := user32.NewProc("DefWindowProcW")
	postQuitMessage := user32.NewProc("PostQuitMessage")
	getMessage := user32.NewProc("GetMessageW")
	translateMessage := user32.NewProc("TranslateMessage")
	dispatchMessage := user32.NewProc("DispatchMessageW")

	instance, _, _ := getModuleHandle.Call(0)
	windowProcedure := windows.NewCallback(func(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
		switch message {
		case wmClose:
			destroyWindow.Call(window)
			return 0
		case 0x0002: // WM_DESTROY
			postQuitMessage.Call(0)
			return 0
		default:
			result, _, _ := defWindowProc.Call(window, uintptr(message), wParam, lParam)
			return result
		}
	})
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProc: windowProcedure,
		Instance: instance, ClassName: windowClassName,
	}
	registered, _, registerErr := registerClass.Call(uintptr(unsafe.Pointer(&class)))
	if registered == 0 {
		return fmt.Errorf("register hidden window class: %w", registerErr)
	}
	window, _, createErr := createWindow.Call(
		0, uintptr(unsafe.Pointer(windowClassName)), uintptr(unsafe.Pointer(windowTitle)), 0,
		0, 0, 0, 0, 0, 0, instance, 0,
	)
	if window == 0 {
		return fmt.Errorf("create hidden helper window: %w", createErr)
	}
	if err := os.WriteFile(readyPath, []byte("ready"), 0o600); err != nil {
		return err
	}

	var message windowMessage
	for {
		result, _, callErr := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("read hidden window message: %w", callErr)
		}
		if result == 0 {
			return nil
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
}
