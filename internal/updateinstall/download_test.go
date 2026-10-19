package updateinstall

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meow-launcher/internal/updatecontract"
)

type recordingVerifier struct {
	err   error
	paths []string
}

func (v *recordingVerifier) Verify(_ context.Context, path string, _ updatecontract.Signature) error {
	v.paths = append(v.paths, path)
	return v.err
}

func TestDownloaderVerifiesHashAndSignatureBeforePublishingFile(t *testing.T) {
	payload := []byte("signed installer bytes")
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	verifier := &recordingVerifier{}
	release := validRelease(server.URL+"/DevHelperSetup.exe", payload)
	destination := filepath.Join(t.TempDir(), "downloads")

	result, err := (Downloader{HTTPClient: server.Client(), MaxBytes: 1024, VerifySigner: verifier}).Download(context.Background(), release, destination)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(result.Path) != "dev-helper-1.2.3.exe" || !result.SignatureOkay || len(verifier.paths) != 1 {
		t.Fatalf("result=%+v verifier=%+v", result, verifier.paths)
	}
	if got, err := os.ReadFile(result.Path); err != nil || string(got) != string(payload) {
		t.Fatalf("downloaded=%q err=%v", got, err)
	}
	partials, _ := filepath.Glob(filepath.Join(destination, "*.partial"))
	if len(partials) != 0 {
		t.Fatalf("partial files remain: %v", partials)
	}
}

func TestDownloaderReusesVerifiedCachedInstallerWithoutNetworkRequest(t *testing.T) {
	payload := []byte("already downloaded installer")
	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requestCount++
		http.Error(response, "network must not be used", http.StatusInternalServerError)
	}))
	defer server.Close()
	release := validRelease(server.URL+"/setup.exe", payload)
	destination := filepath.Join(t.TempDir(), "downloads")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	cachedPath := filepath.Join(destination, "dev-helper-1.2.3.exe")
	if err := os.WriteFile(cachedPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	verifier := &recordingVerifier{}
	var progress []DownloadProgress

	result, err := (Downloader{
		HTTPClient: server.Client(), VerifySigner: verifier,
		Progress: func(current DownloadProgress) { progress = append(progress, current) },
	}).Download(context.Background(), release, destination)
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 0 {
		t.Fatalf("network requests=%d want=0", requestCount)
	}
	if result.Path != cachedPath || result.Bytes != int64(len(payload)) || len(verifier.paths) != 1 {
		t.Fatalf("result=%+v verifier=%v", result, verifier.paths)
	}
	if len(progress) != 2 || progress[0].Phase != "verifying" || progress[1].Phase != "complete" {
		t.Fatalf("progress=%#v", progress)
	}
}

func TestDownloaderReplacesInvalidCachedInstaller(t *testing.T) {
	payload := []byte("fresh installer")
	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requestCount++
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	release := validRelease(server.URL+"/setup.exe", payload)
	destination := filepath.Join(t.TempDir(), "downloads")
	if err := os.MkdirAll(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	cachedPath := filepath.Join(destination, "dev-helper-1.2.3.exe")
	if err := os.WriteFile(cachedPath, []byte("tampered cache"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := (Downloader{HTTPClient: server.Client(), VerifySigner: &recordingVerifier{}}).Download(context.Background(), release, destination)
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 || result.Path != cachedPath {
		t.Fatalf("network requests=%d result=%+v", requestCount, result)
	}
	got, readErr := os.ReadFile(cachedPath)
	if readErr != nil || string(got) != string(payload) {
		t.Fatalf("cached installer=%q err=%v", got, readErr)
	}
}

func TestDownloaderHashMismatchLeavesNoInstallableFile(t *testing.T) {
	payload := []byte("tampered")
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	release := validRelease(server.URL+"/setup.exe", []byte("expected"))
	destination := filepath.Join(t.TempDir(), "downloads")
	_, err := (Downloader{HTTPClient: server.Client(), VerifySigner: &recordingVerifier{}}).Download(context.Background(), release, destination)
	if CodeOf(err) != "HASH_MISMATCH" {
		t.Fatalf("error=%v code=%s", err, CodeOf(err))
	}
	entries, readErr := os.ReadDir(destination)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("download directory=%v err=%v", entries, readErr)
	}
}

func TestDownloaderEnforcesStreamingSizeLimit(t *testing.T) {
	payload := []byte(strings.Repeat("x", 32))
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Del("Content-Length")
		if flusher, ok := response.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	release := validRelease(server.URL+"/setup.exe", payload)
	_, err := (Downloader{HTTPClient: server.Client(), MaxBytes: 8, VerifySigner: &recordingVerifier{}}).Download(context.Background(), release, filepath.Join(t.TempDir(), "downloads"))
	if CodeOf(err) != "DOWNLOAD_TOO_LARGE" {
		t.Fatalf("error=%v code=%s", err, CodeOf(err))
	}
}

func TestDownloaderSignatureFailureLeavesNoInstallableFile(t *testing.T) {
	payload := []byte("unsigned")
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "downloads")
	verifier := &recordingVerifier{err: newError("SIGNATURE_INVALID", "bad signature", nil)}
	_, err := (Downloader{HTTPClient: server.Client(), VerifySigner: verifier}).Download(context.Background(), validRelease(server.URL+"/setup.exe", payload), destination)
	if CodeOf(err) != "SIGNATURE_INVALID" {
		t.Fatalf("error=%v", err)
	}
	entries, readErr := os.ReadDir(destination)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("download directory=%v err=%v", entries, readErr)
	}
}

func TestDownloaderStopsAConnectionThatNeverDeliversBodyBytes(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "MeowLauncher-Update/1.0" || request.Header.Get("Accept") != "application/octet-stream" {
			t.Errorf("unexpected download headers: user-agent=%q accept=%q", request.Header.Get("User-Agent"), request.Header.Get("Accept"))
		}
		if flusher, ok := response.(http.Flusher); ok {
			flusher.Flush()
		}
		<-request.Context().Done()
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "downloads")
	release := validRelease(server.URL+"/setup.exe", []byte("never delivered"))

	started := time.Now()
	_, err := (Downloader{
		HTTPClient:   server.Client(),
		IdleTimeout:  60 * time.Millisecond,
		VerifySigner: &recordingVerifier{},
	}).Download(context.Background(), release, destination)
	if CodeOf(err) != "DOWNLOAD_STALLED" {
		t.Fatalf("error=%v code=%s", err, CodeOf(err))
	}
	if time.Since(started) > time.Second {
		t.Fatalf("stalled download took too long: %s", time.Since(started))
	}
	entries, readErr := os.ReadDir(destination)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("download directory=%v err=%v", entries, readErr)
	}
}

func TestDownloaderKeepsAliveWhileBytesContinueArriving(t *testing.T) {
	payload := []byte("slow-but-progressing")
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		flusher, _ := response.(http.Flusher)
		for _, value := range payload {
			_, _ = response.Write([]byte{value})
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(15 * time.Millisecond)
		}
	}))
	defer server.Close()
	result, err := (Downloader{
		HTTPClient:   server.Client(),
		IdleTimeout:  60 * time.Millisecond,
		VerifySigner: &recordingVerifier{},
	}).Download(context.Background(), validRelease(server.URL+"/setup.exe", payload), filepath.Join(t.TempDir(), "downloads"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Bytes != int64(len(payload)) {
		t.Fatalf("downloaded bytes=%d want=%d", result.Bytes, len(payload))
	}
}

func TestDownloaderReportsRealByteProgressAndVerification(t *testing.T) {
	payload := []byte(strings.Repeat("progress", 128))
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	var progress []DownloadProgress
	_, err := (Downloader{
		HTTPClient: server.Client(), VerifySigner: &recordingVerifier{},
		Progress: func(current DownloadProgress) { progress = append(progress, current) },
	}).Download(context.Background(), validRelease(server.URL+"/setup.exe", payload), filepath.Join(t.TempDir(), "downloads"))
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) < 4 || progress[0].Phase != "downloading" || progress[0].Bytes != 0 || progress[0].TotalBytes != int64(len(payload)) {
		t.Fatalf("unexpected initial progress: %#v", progress)
	}
	last := progress[len(progress)-1]
	if last.Phase != "complete" || last.Bytes != int64(len(payload)) || last.TotalBytes != int64(len(payload)) {
		t.Fatalf("unexpected final progress: %#v", progress)
	}
	foundVerification := false
	for _, current := range progress {
		if current.Phase == "verifying" && current.Bytes == int64(len(payload)) {
			foundVerification = true
		}
	}
	if !foundVerification {
		t.Fatalf("verification progress missing: %#v", progress)
	}
}

func validRelease(downloadURL string, payload []byte) updatecontract.ProductRelease {
	return updatecontract.ProductRelease{
		ProductID: "dev-helper", Channel: "stable", Version: "1.2.3", AccessPolicy: "public",
		PublishedAt:  time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		ReleaseNotes: "test", DownloadURL: downloadURL, InstallerType: "nsis",
		InstallScope: "per-user", InstallDirectoryName: "Dev Helper", ExecutableRelativePath: "Dev Helper.exe",
		SHA256:                 fmt.Sprintf("%x", sha256.Sum256(payload)),
		Signature:              updatecontract.Signature{Method: "authenticode", Required: true, Publisher: "Meow Test Publisher"},
		MinimumLauncherVersion: "0.6.6", MinimumSupportedVersion: "0.1.0",
		DataMigrationVersion: "none",
		Rollback:             updatecontract.Rollback{Strategy: "file-backup", PreviousVersionSupported: true, BackupRequired: true},
	}
}
