//go:build launcher

package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"meow-launcher/internal/settings"
)

//go:embed all:launcherfrontend/dist
var launcherAssets embed.FS

func launcherNoAssetCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func main() {
	app := NewLauncherApp()
	dataRoot, err := settings.DataRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Meow Launcher failed:", err)
		os.Exit(2)
	}
	err = wails.Run(&options.App{
		Title:     "Meow Launcher",
		Width:     1060,
		Height:    700,
		MinWidth:  900,
		MinHeight: 600,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets:     launcherAssets,
			Middleware: launcherNoAssetCache,
		},
		BackgroundColour: &options.RGBA{R: 14, G: 14, B: 13, A: 255},
		OnStartup:        app.startup,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "meow-launcher-lifecycle-v1",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				app.handleSecondInstanceLaunch(data.Args)
			},
		},
		Bind: []interface{}{app},
		Windows: &windows.Options{
			Theme:               windows.Dark,
			WebviewUserDataPath: filepath.Join(dataRoot, "webview2-launcher"),
			WindowClassName:     "MeowLauncherLifecycleWindow",
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Meow Launcher failed:", err)
	}
}
