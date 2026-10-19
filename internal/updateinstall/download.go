package updateinstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"meow-launcher/internal/updatecontract"
)

const defaultMaximumDownloadBytes int64 = 1 << 30
const defaultDownloadIdleTimeout = 45 * time.Second

var safeFilenamePart = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type SignatureVerifier interface {
	Verify(context.Context, string, updatecontract.Signature) error
}

type Downloader struct {
	HTTPClient   *http.Client
	MaxBytes     int64
	IdleTimeout  time.Duration
	VerifySigner SignatureVerifier
	Progress     func(DownloadProgress)
}

type DownloadProgress struct {
	Phase      string `json:"phase"`
	Bytes      int64  `json:"bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

type DownloadResult struct {
	Path          string `json:"path"`
	Bytes         int64  `json:"bytes"`
	SHA256        string `json:"sha256"`
	SignatureOkay bool   `json:"signature_okay"`
}

func (d Downloader) Download(ctx context.Context, release updatecontract.ProductRelease, destinationDirectory string) (DownloadResult, error) {
	if err := release.Validate(); err != nil {
		return DownloadResult{}, newError("RELEASE_INVALID", "发布记录无效，不能下载安装", err)
	}
	if !filepath.IsAbs(destinationDirectory) {
		return DownloadResult{}, newError("DOWNLOAD_PATH_INVALID", "下载目录必须是绝对路径", nil)
	}
	parsed, _ := url.Parse(release.DownloadURL)
	if parsed == nil || !strings.EqualFold(parsed.Scheme, "https") {
		return DownloadResult{}, newError("DOWNLOAD_INSECURE", "安装包必须通过 HTTPS 下载", nil)
	}
	if err := os.MkdirAll(destinationDirectory, 0o700); err != nil {
		return DownloadResult{}, newError("DOWNLOAD_FAILED", "无法创建 Launcher 下载目录", err)
	}
	maximum := d.MaxBytes
	if maximum <= 0 {
		maximum = defaultMaximumDownloadBytes
	}
	finalPath := filepath.Join(destinationDirectory, downloadFilename(release, parsed))
	if cached, reused, err := d.reuseCachedDownload(ctx, release, finalPath, maximum); err != nil {
		return DownloadResult{}, err
	} else if reused {
		return cached, nil
	}
	if d.HTTPClient == nil && preferPlatformDownload(parsed) {
		temporaryPath, written, total, err := platformDownload(ctx, release.DownloadURL, destinationDirectory, maximum, func(written, total int64) {
			d.reportProgress("downloading", written, total)
		})
		if err != nil {
			return DownloadResult{}, err
		}
		defer os.Remove(temporaryPath)
		if total <= 0 {
			total = written
		}
		d.reportProgress("verifying", written, total)
		return d.verifyAndFinalize(ctx, release, parsed, destinationDirectory, temporaryPath, written, total)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return DownloadResult{}, newError("DOWNLOAD_FAILED", "无法创建安装包下载请求", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "MeowLauncher-Update/1.0")
	client := d.HTTPClient
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = platformProxy
		transport.ForceAttemptHTTP2 = false
		transport.ResponseHeaderTimeout = 30 * time.Second
		client = &http.Client{Timeout: 30 * time.Minute, Transport: transport}
	}
	response, err := client.Do(request)
	if err != nil {
		return DownloadResult{}, newError("NETWORK_FAILED", "无法下载安装包："+networkErrorDetail(err), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return DownloadResult{}, newError("NETWORK_FAILED", fmt.Sprintf("安装包下载返回 HTTP %d", response.StatusCode), nil)
	}
	if response.ContentLength > maximum {
		return DownloadResult{}, newError("DOWNLOAD_TOO_LARGE", "安装包超过 Launcher 允许的大小", nil)
	}
	d.reportProgress("downloading", 0, response.ContentLength)
	temporary, err := os.CreateTemp(destinationDirectory, ".meow-download-*.partial")
	if err != nil {
		return DownloadResult{}, newError("DOWNLOAD_FAILED", "无法创建临时下载文件", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	idleTimeout := d.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = defaultDownloadIdleTimeout
	}
	written, copyErr := copyWithIdleTimeoutAndProgress(ctx, temporary, io.LimitReader(response.Body, maximum+1), response.Body, idleTimeout, func(written int64) {
		d.reportProgress("downloading", written, response.ContentLength)
	})
	if copyErr == nil && written > maximum {
		copyErr = newError("DOWNLOAD_TOO_LARGE", "安装包超过 Launcher 允许的大小", nil)
	}
	if syncErr := temporary.Sync(); copyErr == nil {
		copyErr = syncErr
	}
	if closeErr := temporary.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		if CodeOf(copyErr) == "DOWNLOAD_TOO_LARGE" || CodeOf(copyErr) == "DOWNLOAD_STALLED" {
			return DownloadResult{}, copyErr
		}
		return DownloadResult{}, newError("DOWNLOAD_FAILED", "安装包下载未完整写入磁盘", copyErr)
	}
	d.reportProgress("verifying", written, response.ContentLength)
	return d.verifyAndFinalize(ctx, release, parsed, destinationDirectory, temporaryPath, written, response.ContentLength)
}

func (d Downloader) reuseCachedDownload(ctx context.Context, release updatecontract.ProductRelease, finalPath string, maximum int64) (DownloadResult, bool, error) {
	info, err := os.Lstat(finalPath)
	if errors.Is(err, os.ErrNotExist) {
		return DownloadResult{}, false, nil
	}
	if err != nil {
		return DownloadResult{}, false, newError("DOWNLOAD_FAILED", "无法检查本机已有的安装包", err)
	}

	if info.Mode().IsRegular() && info.Size() <= maximum {
		d.reportProgress("verifying", info.Size(), info.Size())
		if hashErr := updatecontract.VerifyFileSHA256(finalPath, release.SHA256); hashErr == nil {
			verifier := d.VerifySigner
			if verifier == nil {
				verifier = PlatformSignatureVerifier{}
			}
			if signatureErr := verifier.Verify(ctx, finalPath, release.Signature); signatureErr != nil {
				return DownloadResult{}, false, signatureErr
			}
			d.reportProgress("complete", info.Size(), info.Size())
			return DownloadResult{
				Path:          finalPath,
				Bytes:         info.Size(),
				SHA256:        strings.ToLower(release.SHA256),
				SignatureOkay: true,
			}, true, nil
		}
	}

	// The path is fully controlled by Launcher and only contains a cached
	// installer for this exact product/version. A stale, partial or tampered
	// cache entry must not make every retry fail forever; remove that one file
	// and fetch a fresh copy.
	if removeErr := os.Remove(finalPath); removeErr != nil {
		return DownloadResult{}, false, newError("DOWNLOAD_CONFLICT", "本机已有的安装包无效，且无法替换；请关闭占用该文件的程序后重试", removeErr)
	}
	return DownloadResult{}, false, nil
}

func (d Downloader) verifyAndFinalize(ctx context.Context, release updatecontract.ProductRelease, parsed *url.URL, destinationDirectory, temporaryPath string, written, total int64) (DownloadResult, error) {
	if err := updatecontract.VerifyFileSHA256(temporaryPath, release.SHA256); err != nil {
		return DownloadResult{}, newError("HASH_MISMATCH", "HASH_MISMATCH：下载文件与发布清单不一致", err)
	}
	verifier := d.VerifySigner
	if verifier == nil {
		verifier = PlatformSignatureVerifier{}
	}
	if err := verifier.Verify(ctx, temporaryPath, release.Signature); err != nil {
		return DownloadResult{}, err
	}
	filename := downloadFilename(release, parsed)
	finalPath := filepath.Join(destinationDirectory, filename)
	if _, err := os.Lstat(finalPath); err == nil {
		if hashErr := updatecontract.VerifyFileSHA256(finalPath, release.SHA256); hashErr == nil {
			if signatureErr := verifier.Verify(ctx, finalPath, release.Signature); signatureErr == nil {
				d.reportProgress("complete", written, total)
				return DownloadResult{Path: finalPath, Bytes: written, SHA256: strings.ToLower(release.SHA256), SignatureOkay: true}, nil
			}
		}
		return DownloadResult{}, newError("DOWNLOAD_CONFLICT", "下载目录中已有同名但内容不同的文件", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return DownloadResult{}, newError("DOWNLOAD_FAILED", "无法检查最终安装包路径", err)
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return DownloadResult{}, newError("DOWNLOAD_FAILED", "无法完成安装包下载", err)
	}
	d.reportProgress("complete", written, total)
	return DownloadResult{Path: finalPath, Bytes: written, SHA256: strings.ToLower(release.SHA256), SignatureOkay: true}, nil
}

func (d Downloader) reportProgress(phase string, written, total int64) {
	if d.Progress != nil {
		d.Progress(DownloadProgress{Phase: phase, Bytes: written, TotalBytes: total})
	}
}

type downloadCopyResult struct {
	written int64
	err     error
}

type activityWriter struct {
	destination io.Writer
	written     atomic.Int64
	lastWrite   atomic.Int64
	progress    func(int64)
}

func newActivityWriter(destination io.Writer) *activityWriter {
	writer := &activityWriter{destination: destination}
	writer.lastWrite.Store(time.Now().UnixNano())
	return writer
}

func (w *activityWriter) Write(payload []byte) (int, error) {
	written, err := w.destination.Write(payload)
	if written > 0 {
		total := w.written.Add(int64(written))
		w.lastWrite.Store(time.Now().UnixNano())
		if w.progress != nil {
			w.progress(total)
		}
	}
	return written, err
}

func copyWithIdleTimeout(ctx context.Context, destination io.Writer, source io.Reader, closer io.Closer, idleTimeout time.Duration) (int64, error) {
	return copyWithIdleTimeoutAndProgress(ctx, destination, source, closer, idleTimeout, nil)
}

func copyWithIdleTimeoutAndProgress(ctx context.Context, destination io.Writer, source io.Reader, closer io.Closer, idleTimeout time.Duration, progress func(int64)) (int64, error) {
	writer := newActivityWriter(destination)
	writer.progress = progress
	completed := make(chan downloadCopyResult, 1)
	go func() {
		written, err := io.Copy(writer, source)
		completed <- downloadCopyResult{written: written, err: err}
	}()

	checkEvery := idleTimeout / 4
	if checkEvery < 10*time.Millisecond {
		checkEvery = 10 * time.Millisecond
	}
	if checkEvery > time.Second {
		checkEvery = time.Second
	}
	ticker := time.NewTicker(checkEvery)
	defer ticker.Stop()
	for {
		select {
		case result := <-completed:
			return result.written, result.err
		case <-ctx.Done():
			_ = closer.Close()
			<-completed
			return writer.written.Load(), ctx.Err()
		case <-ticker.C:
			lastWrite := time.Unix(0, writer.lastWrite.Load())
			if time.Since(lastWrite) < idleTimeout {
				continue
			}
			_ = closer.Close()
			<-completed
			return writer.written.Load(), newError("DOWNLOAD_STALLED", "下载连接长时间没有收到数据，已安全停止；请检查网络后重试", nil)
		}
	}
}

func downloadFilename(release updatecontract.ProductRelease, parsed *url.URL) string {
	extension := filepath.Ext(parsed.Path)
	if len(extension) > 12 || safeFilenamePart.ReplaceAllString(extension, "") != extension {
		extension = ".bin"
	}
	base := safeFilenamePart.ReplaceAllString(release.ProductID+"-"+release.Version, "-")
	return strings.Trim(base, ".-") + extension
}
