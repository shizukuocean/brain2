//go:build windows

package agentconnect

import (
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const appModelPackagesKey = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

// findCodexDesktopCommand reads the persistent Windows AppX repository rather
// than relying on PATH entries injected into a particular Codex process. Those
// temporary entries disappear when Launcher is started normally after reboot.
func findCodexDesktopCommand() string {
	packages, err := registry.OpenKey(registry.CURRENT_USER, appModelPackagesKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return ""
	}
	defer packages.Close()

	names, err := packages.ReadSubKeyNames(-1)
	if err != nil {
		return ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	roots := make([]string, 0, 2)
	for _, name := range names {
		if !strings.HasPrefix(strings.ToLower(name), "openai.codex_") {
			continue
		}
		item, openErr := registry.OpenKey(packages, name, registry.QUERY_VALUE)
		if openErr != nil {
			continue
		}
		root, _, valueErr := item.GetStringValue("PackageRootFolder")
		item.Close()
		if valueErr == nil && strings.TrimSpace(root) != "" {
			roots = append(roots, root)
		}
	}
	return findPackagedCodexExecutable(roots)
}
