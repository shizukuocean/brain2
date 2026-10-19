//go:build !windows

package agentconnect

import "os/exec"

func setHidden(command *exec.Cmd) {}
