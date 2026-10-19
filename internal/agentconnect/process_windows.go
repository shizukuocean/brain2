//go:build windows

package agentconnect

import (
	"os/exec"
	"syscall"
)

func setHidden(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
