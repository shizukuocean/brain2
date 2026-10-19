//go:build windows

package agentconnect

import (
	"context"
	"os"
	"testing"
)

func TestFindCodexDesktopCommandFromInstalledAppX(t *testing.T) {
	installationEvidence := findCodexDesktopCommand()
	if installationEvidence == "" {
		t.Skip("Codex Desktop AppX is not installed for the current Windows user")
	}
	info, err := os.Stat(installationEvidence)
	if err != nil {
		t.Fatalf("Codex Desktop installation evidence is unavailable: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("Codex Desktop installation evidence is not a file: %s", installationEvidence)
	}
	t.Log("Codex Desktop installation was detected without treating its packaged binary as a CLI")
}

func TestDetectAllNeverReusesPackagedCodexAsCLI(t *testing.T) {
	var cli, desktop Detection
	for _, item := range DetectAll(context.Background()) {
		switch item.ID {
		case "codex":
			cli = item
		case "codex-desktop":
			desktop = item
		}
	}
	if desktop.Detected && cli.Detected && sameExecutable(cli.Executable, desktop.Executable) {
		t.Fatalf("packaged Codex Desktop binary was reused as CLI: %s", cli.Executable)
	}
	if desktop.Detected && !desktop.QuickConnect {
		t.Fatal("Codex Desktop should use the user-config quick-connect adapter")
	}
}
