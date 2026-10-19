//go:build !windows

package updatecontract

import (
	"context"
	"errors"
	"net/url"
)

func preferPlatformManifestRead(*url.URL) bool { return false }

func platformManifestRead(context.Context, string, int64) ([]byte, error) {
	return nil, errors.New("当前平台不支持系统 HTTPS 清单读取")
}
