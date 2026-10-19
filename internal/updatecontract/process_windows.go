package updatecontract

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// keepManifestCommandInBackground keeps the Windows HTTPS helper behind the
// already-visible Launcher window so an update check cannot steal focus.
func keepManifestCommandInBackground(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
