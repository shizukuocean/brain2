//go:build !windows

package updateinstall

import (
	"context"
	"net/url"
)

func preferPlatformDownload(*url.URL) bool { return false }

func platformDownload(context.Context, string, string, int64, func(int64, int64)) (string, int64, int64, error) {
	return "", 0, 0, newError("NETWORK_FAILED", "当前平台不支持系统 HTTPS 下载", nil)
}
