//go:build !launcher

package main

import (
	"embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"meow-launcher/internal/brain2"
	"meow-launcher/internal/settings"
)

//go:embed all:frontend/dist
var assets embed.FS

const obsidianAcceptanceMode = "simulate-no-obsidian"

// buildAcceptanceMode is empty for normal releases. Acceptance-only binaries may
// set it with -ldflags without changing or overwriting the normal Launcher build.
var buildAcceptanceMode string

func noAssetCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mcp-serve" {
		mode, target, err := mcpTargetFromArgs(os.Args[2:])
		if err == nil && mode == "registry" {
			err = brain2.ServeMCPRegistry(target, os.Stdin, os.Stdout)
		} else if err == nil {
			err = brain2.ServeMCP(target, os.Stdin, os.Stdout)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Brain2 MCP failed:", err)
			os.Exit(1)
		}
		return
	}
	dataRootOverride, profileName, err := launcherProfileFromArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Meow Launcher failed:", err)
		os.Exit(2)
	}
	if buildAcceptanceMode == obsidianAcceptanceMode {
		machineRoot, rootErr := settings.MachineRoot()
		if rootErr != nil {
			fmt.Fprintln(os.Stderr, "Meow Launcher acceptance profile failed:", rootErr)
			os.Exit(2)
		}
		dataRootOverride, profileName = launcherAcceptanceProfile(dataRootOverride, profileName, machineRoot)
	}
	if dataRootOverride != "" {
		_ = os.Setenv("MEOW_LAUNCHER_DATA_ROOT", dataRootOverride)
	}
	if profileName != "" {
		_ = os.Setenv("MEOW_LAUNCHER_PROFILE", profileName)
	}
	if buildAcceptanceMode == obsidianAcceptanceMode || hasLauncherFlag(os.Args[1:], "--simulate-no-obsidian") {
		_ = os.Setenv("MEOW_LAUNCHER_SIMULATE_NO_OBSIDIAN", "1")
		_ = os.Setenv("MEOW_LAUNCHER_ACCEPTANCE_MODE", obsidianAcceptanceMode)
	}
	app := NewApp()
	dataRoot, _ := settings.DataRoot()

	windowTitle := "Brain2 Manager"
	if buildAcceptanceMode == obsidianAcceptanceMode {
		windowTitle += " · OB 安装验收"
	}
	err = wails.Run(&options.App{
		Title:     windowTitle,
		Width:     1180,
		Height:    760,
		MinWidth:  960,
		MinHeight: 640,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: noAssetCache,
		},
		BackgroundColour: &options.RGBA{
			R: 14,
			G: 14,
			B: 13,
			A: 255,
		},
		OnStartup: app.startup,
		Bind:      []interface{}{app},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(dataRoot, "webview2-brain2-manager"),
			WindowClassName:     "Brain2ManagerV066Window",
		},
	})
	if err != nil {
		println("Meow Launcher failed:", err.Error())
	}
}

func launcherAcceptanceProfile(dataRoot, profileName, machineRoot string) (string, string) {
	if strings.TrimSpace(dataRoot) == "" {
		dataRoot = filepath.Join(machineRoot, "acceptance-ob-install")
	}
	if strings.TrimSpace(profileName) == "" {
		profileName = "OB 安装验收"
	}
	return dataRoot, profileName
}

func hasLauncherFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func launcherProfileFromArgs(args []string) (string, string, error) {
	var dataRoot, profile string
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--data-root":
			if index+1 >= len(args) {
				return "", "", errors.New("--data-root 需要一个绝对路径")
			}
			index++
			dataRoot = strings.TrimSpace(args[index])
			if !filepath.IsAbs(dataRoot) {
				return "", "", errors.New("--data-root 必须是绝对路径")
			}
		case "--profile":
			if index+1 >= len(args) {
				return "", "", errors.New("--profile 需要名称")
			}
			index++
			profile = strings.TrimSpace(args[index])
		}
	}
	if dataRoot != "" {
		dataRoot = filepath.Clean(dataRoot)
	}
	return dataRoot, profile, nil
}

func mcpTargetFromArgs(args []string) (string, string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--registry" && i+1 < len(args) {
			return "registry", args[i+1], nil
		}
		if args[i] == "--root" && i+1 < len(args) {
			return "root", args[i+1], nil
		}
	}
	return "", "", fmt.Errorf("mcp-serve requires --registry <Launcher registry path>")
}
