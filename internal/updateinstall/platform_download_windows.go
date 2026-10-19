package updateinstall

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	platformDownloadAttempts     = 6
	platformDownloadRetryBase    = 500 * time.Millisecond
	platformDownloadPollInterval = 200 * time.Millisecond
)

var runPlatformDownloadAttempt = runCurlDownloadAttempt
var getPlatformDownloadSize = platformDownloadSize
var getPlatformProxyForURL = platformProxyForURL

func preferPlatformDownload(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Hostname())
	return host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

var contentRangeTotal = regexp.MustCompile(`(?im)^content-range:\s*bytes\s+\d+-\d+/(\d+)\s*$`)

func platformDownload(ctx context.Context, sourceURL, destinationDirectory string, maximum int64, progress func(int64, int64)) (string, int64, int64, error) {
	proxy := getPlatformProxyForURL(sourceURL)
	total := getPlatformDownloadSize(ctx, sourceURL, proxy)
	if total > maximum {
		return "", 0, total, newError("DOWNLOAD_TOO_LARGE", "安装包超过 Launcher 允许的大小", nil)
	}
	temporary, err := os.CreateTemp(destinationDirectory, ".meow-download-*.partial")
	if err != nil {
		return "", 0, total, newError("DOWNLOAD_FAILED", "无法创建临时下载文件", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		os.Remove(temporaryPath)
		return "", 0, total, newError("DOWNLOAD_FAILED", "无法创建临时下载文件", err)
	}

	proxies := []string{proxy}
	if proxy != "" {
		proxies = append(proxies, "")
	}
	var written int64
	var lastError error
	for attempt := 0; attempt < platformDownloadAttempts; attempt++ {
		selectedProxy := proxies[attempt%len(proxies)]
		written, lastError = runPlatformDownloadAttempt(ctx, sourceURL, temporaryPath, selectedProxy, total, maximum, progress)
		if lastError == nil {
			return temporaryPath, written, total, nil
		}
		if CodeOf(lastError) == "DOWNLOAD_TOO_LARGE" || ctx.Err() != nil {
			os.Remove(temporaryPath)
			return "", written, total, lastError
		}
		select {
		case <-ctx.Done():
			os.Remove(temporaryPath)
			return "", written, total, newError("NETWORK_FAILED", fmt.Sprintf("下载已停止：%v", ctx.Err()), ctx.Err())
		case <-time.After(time.Duration(attempt+1) * platformDownloadRetryBase):
		}
	}
	os.Remove(temporaryPath)
	return "", written, total, newError("NETWORK_FAILED", "自动重试与断点续传后仍无法下载安装包："+safeCurlError(lastError), lastError)
}

func runCurlDownloadAttempt(ctx context.Context, sourceURL, destination, proxy string, total, maximum int64, progress func(int64, int64)) (int64, error) {
	args := []string{"--fail", "--location", "--silent", "--show-error", "--proto", "=https", "--proto-redir", "=https", "--connect-timeout", "30", "--continue-at", "-", "--output", destination}
	if proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	args = append(args, sourceURL)
	command := exec.CommandContext(ctx, systemCurlPath(), args...)
	keepUpdateCommandInBackground(command)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return fileSize(destination), newError("NETWORK_FAILED", "无法启动 Windows HTTPS 下载组件", err)
	}
	completed := make(chan error, 1)
	go func() { completed <- command.Wait() }()
	ticker := time.NewTicker(platformDownloadPollInterval)
	defer ticker.Stop()
	for {
		select {
		case err := <-completed:
			written := fileSize(destination)
			if progress != nil {
				progress(written, total)
			}
			if written > maximum {
				return written, newError("DOWNLOAD_TOO_LARGE", "安装包超过 Launcher 允许的大小", nil)
			}
			if err != nil {
				detail := strings.TrimSpace(stderr.String())
				if detail == "" {
					detail = err.Error()
				}
				return written, newError("NETWORK_FAILED", detail, err)
			}
			if written == 0 || (total > 0 && written != total) {
				return written, newError("DOWNLOAD_FAILED", "安装包尚未完整下载", nil)
			}
			return written, nil
		case <-ticker.C:
			written := fileSize(destination)
			if written > maximum {
				_ = command.Process.Kill()
				<-completed
				return written, newError("DOWNLOAD_TOO_LARGE", "安装包超过 Launcher 允许的大小", nil)
			}
			if progress != nil {
				progress(written, total)
			}
		case <-ctx.Done():
			terminateProcessTree(command.Process.Pid)
			select {
			case <-completed:
			case <-time.After(5 * time.Second):
				_ = command.Process.Kill()
				<-completed
			}
			return fileSize(destination), newError("NETWORK_FAILED", fmt.Sprintf("下载已停止：%v", ctx.Err()), ctx.Err())
		}
	}
}

func terminateProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	command := exec.Command("taskkill.exe", "/PID", strconv.Itoa(pid), "/T", "/F")
	keepUpdateCommandInBackground(command)
	command.Stdout = nil
	command.Stderr = nil
	_ = command.Run()
}

func platformDownloadSize(ctx context.Context, sourceURL, proxy string) int64 {
	args := []string{"--fail", "--location", "--silent", "--show-error", "--proto", "=https", "--proto-redir", "=https", "--connect-timeout", "30", "--range", "0-0", "--dump-header", "-", "--output", "NUL"}
	if proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	args = append(args, sourceURL)
	command := exec.CommandContext(ctx, systemCurlPath(), args...)
	keepUpdateCommandInBackground(command)
	output, err := command.Output()
	if err != nil {
		return 0
	}
	match := contentRangeTotal.FindSubmatch(output)
	if len(match) != 2 {
		return 0
	}
	total, err := strconv.ParseInt(string(match[1]), 10, 64)
	if err != nil || total <= 0 {
		return 0
	}
	return total
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func safeCurlError(err error) string {
	if err == nil {
		return "网络连接失败"
	}
	detail := strings.TrimSpace(err.Error())
	if len(detail) > 180 {
		detail = detail[:180] + "…"
	}
	return detail
}

func systemCurlPath() string {
	root := strings.TrimSpace(os.Getenv("SystemRoot"))
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "curl.exe")
}
