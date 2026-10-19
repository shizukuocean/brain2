//go:build windows

package updateinstall

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestPlatformDownloadRetriesAndResumesTheSamePartialFile(t *testing.T) {
	original := runPlatformDownloadAttempt
	originalSize := getPlatformDownloadSize
	originalProxy := getPlatformProxyForURL
	t.Cleanup(func() {
		runPlatformDownloadAttempt = original
		getPlatformDownloadSize = originalSize
		getPlatformProxyForURL = originalProxy
	})

	payload := []byte("complete-installer-payload")
	getPlatformDownloadSize = func(context.Context, string, string) int64 { return int64(len(payload)) }
	getPlatformProxyForURL = func(string) string { return "http://127.0.0.1:7890" }
	var paths []string
	var startingSizes []int64
	var proxies []string
	runPlatformDownloadAttempt = func(_ context.Context, _ string, destination, proxy string, total, maximum int64, progress func(int64, int64)) (int64, error) {
		paths = append(paths, destination)
		startingSizes = append(startingSizes, fileSize(destination))
		proxies = append(proxies, proxy)
		file, err := os.OpenFile(destination, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		defer file.Close()
		if len(paths) == 1 {
			_, _ = file.Write(payload[:8])
			return fileSize(destination), newError("NETWORK_FAILED", "simulated connection drop", nil)
		}
		_, _ = file.Write(payload[8:])
		if progress != nil {
			progress(fileSize(destination), int64(len(payload)))
		}
		return fileSize(destination), nil
	}

	path, written, total, err := platformDownload(context.Background(), "https://github.com/example/release.exe", t.TempDir(), 1024, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if len(paths) != 2 || paths[0] != paths[1] || startingSizes[0] != 0 || startingSizes[1] != 8 {
		t.Fatalf("attempt paths=%v startingSizes=%v", paths, startingSizes)
	}
	if len(proxies) != 2 || proxies[0] == proxies[1] {
		t.Fatalf("expected proxy/direct fallback, got %v", proxies)
	}
	if written != int64(len(payload)) || (total > 0 && total != written) {
		t.Fatalf("written=%d total=%d", written, total)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != string(payload) {
		t.Fatalf("payload=%q err=%v", got, readErr)
	}
}

func TestRunCurlDownloadAttemptCancelsTheWholeProcessTree(t *testing.T) {
	if _, err := exec.LookPath("taskkill.exe"); err != nil {
		t.Skip("taskkill.exe unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	destination := t.TempDir() + `\cancel.partial`
	_, err := runCurlDownloadAttempt(ctx, "https://github.com/example/never-used.exe", destination, "http://127.0.0.1:1", 0, 1024, nil)
	if CodeOf(err) != "NETWORK_FAILED" {
		t.Fatalf("error=%v code=%s", err, CodeOf(err))
	}
}

func TestUpdateHelperProcessesStayHidden(t *testing.T) {
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	keepUpdateCommandInBackground(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow || command.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("update helper process can create a visible console: %#v", command.SysProcAttr)
	}
}
