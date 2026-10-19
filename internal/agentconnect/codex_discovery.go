package agentconnect

import (
	"os"
	"path/filepath"
)

// findPackagedCodexExecutable is platform-neutral so the package discovery
// rules remain testable even when Launcher is cross-compiled off Windows.
func findPackagedCodexExecutable(packageRoots []string) string {
	for _, root := range packageRoots {
		for _, relative := range []string{
			filepath.Join("app", "resources", "codex.exe"),
			filepath.Join("resources", "codex.exe"),
		} {
			candidate := filepath.Join(root, relative)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate
			}
		}
	}
	return ""
}
