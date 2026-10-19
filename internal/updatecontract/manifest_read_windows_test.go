//go:build windows

package updatecontract

import (
	"os/exec"
	"testing"
)

func TestManifestHelperProcessStaysHidden(t *testing.T) {
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	keepManifestCommandInBackground(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow || command.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("manifest helper process can create a visible console: %#v", command.SysProcAttr)
	}
}
