//go:build windows

package main

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func platformObsidianCandidates() []string {
	candidates := []string{}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, appPath := range []string{
			`Software\Microsoft\Windows\CurrentVersion\App Paths\Obsidian.exe`,
			`Software\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths\Obsidian.exe`,
		} {
			if key, err := registry.OpenKey(root, appPath, registry.QUERY_VALUE); err == nil {
				if value, _, valueErr := key.GetStringValue(""); valueErr == nil {
					if executable := registryExecutable(value); executable != "" {
						candidates = append(candidates, executable)
					}
				}
				_ = key.Close()
			}
		}
		for _, uninstallPath := range []string{
			`Software\Microsoft\Windows\CurrentVersion\Uninstall`,
			`Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
		} {
			candidates = append(candidates, uninstallObsidianCandidates(root, uninstallPath)...)
		}
	}
	return candidates
}

func uninstallObsidianCandidates(root registry.Key, uninstallPath string) []string {
	key, err := registry.OpenKey(root, uninstallPath, registry.READ)
	if err != nil {
		return nil
	}
	defer key.Close()

	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	candidates := []string{}
	for _, name := range names {
		entry, openErr := registry.OpenKey(key, name, registry.QUERY_VALUE)
		if openErr != nil {
			continue
		}
		displayName, _, _ := entry.GetStringValue("DisplayName")
		if !strings.Contains(strings.ToLower(displayName), "obsidian") {
			_ = entry.Close()
			continue
		}
		if displayIcon, _, iconErr := entry.GetStringValue("DisplayIcon"); iconErr == nil {
			if executable := registryExecutable(displayIcon); executable != "" {
				candidates = append(candidates, executable)
			}
		}
		if installLocation, _, locationErr := entry.GetStringValue("InstallLocation"); locationErr == nil && strings.TrimSpace(installLocation) != "" {
			candidates = append(candidates, filepath.Join(strings.Trim(strings.TrimSpace(installLocation), `"`), "Obsidian.exe"))
		}
		_ = entry.Close()
	}
	return candidates
}

func registryExecutable(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, `"`) {
		if end := strings.Index(value[1:], `"`); end >= 0 {
			value = value[1 : end+1]
		}
	} else if comma := strings.Index(value, ","); comma >= 0 {
		value = value[:comma]
	}
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if !strings.EqualFold(filepath.Ext(value), ".exe") {
		return ""
	}
	return filepath.Clean(value)
}
