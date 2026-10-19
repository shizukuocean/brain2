package updateinstall

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// keepUpdateCommandInBackground prevents curl, PowerShell and taskkill helpers
// from creating a console window or stealing focus from Launcher. These are
// implementation details of the visible Launcher operation, not separate apps.
func keepUpdateCommandInBackground(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
