package updatecontract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func preferPlatformManifestRead(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Hostname())
	return host == "github.com" || strings.HasSuffix(host, ".github.io") || strings.HasSuffix(host, ".githubusercontent.com")
}

func platformManifestRead(ctx context.Context, source string, maximum int64) ([]byte, error) {
	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, systemCurlPath(),
		"--fail", "--location", "--silent", "--show-error",
		"--proto", "=https", "--proto-redir", "=https",
		"--connect-timeout", "10", "--max-time", "30",
		"--max-filesize", strconv.FormatInt(maximum, 10), source,
	)
	keepManifestCommandInBackground(command)
	command.Stderr = &stderr
	data, err := command.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, errors.New(detail)
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("更新清单超过 %d 字节上限", maximum)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("更新清单为空")
	}
	return data, nil
}

func systemCurlPath() string {
	root := strings.TrimSpace(os.Getenv("SystemRoot"))
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "curl.exe")
}
