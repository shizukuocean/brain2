package processcontrol

import (
	"context"
	"fmt"
	"path/filepath"
)

// CloseExecutable asks every running instance at executablePath to close
// normally, then waits for those exact processes to exit. It never force-kills
// a product: callers can stop an update and ask the user to close it manually.
func CloseExecutable(ctx context.Context, executablePath string) error {
	if executablePath == "" {
		return fmt.Errorf("目标应用路径为空")
	}
	absolutePath, err := filepath.Abs(executablePath)
	if err != nil {
		return fmt.Errorf("无法解析目标应用路径: %w", err)
	}
	return closeExecutable(ctx, filepath.Clean(absolutePath))
}
