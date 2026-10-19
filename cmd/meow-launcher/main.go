//go:build windows

package main

import (
	"runtime"

	"meow-launcher/internal/ui"
)

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_ = ui.Run()
}
