//go:build !windows

package processcontrol

import (
	"context"
	"fmt"
)

func closeExecutable(context.Context, string) error {
	return fmt.Errorf("当前平台暂不支持安全关闭运行中的桌面产品")
}
