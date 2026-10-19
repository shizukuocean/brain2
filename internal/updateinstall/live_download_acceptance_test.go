package updateinstall

import (
	"context"
	"os"
	"testing"
	"time"

	"meow-launcher/internal/updatecontract"
)

func TestLiveHelperDownloadThroughPlatformProxy(t *testing.T) {
	if os.Getenv("MEOW_LIVE_HELPER_DOWNLOAD_TEST") != "1" {
		t.Skip("set MEOW_LIVE_HELPER_DOWNLOAD_TEST=1 to run the full Helper download")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	release := updatecontract.ProductRelease{
		ProductID: "dev-helper", Channel: "preview", Version: "0.3.1-preview.2", PublishedAt: "2026-08-11T12:33:32Z",
		ReleaseNotes: "live Helper acceptance", DownloadURL: "https://github.com/shizukuocean/meow-releases/releases/download/updates-preview-2026-08-11.2/dev-helper-0.3.1-preview.2-windows-x64.exe",
		InstallerType: "meow-bundle", AccessPolicy: "public", InstallScope: "per-user", InstallDirectoryName: "Dev Helper", ExecutableRelativePath: "Dev Helper.exe",
		SHA256: "7ecd434102096fcfe2d8de9321127c7b52273cd1be90286b4864648f5a6713fd", Signature: updatecontract.Signature{Method: "authenticode", Required: false},
		MinimumLauncherVersion: "0.6.6", MinimumSupportedVersion: "0.3.0-p1", DataMigrationVersion: "none", Rollback: updatecontract.Rollback{Strategy: "bundle-transaction", PreviousVersionSupported: true, BackupRequired: true},
	}
	var lastProgress DownloadProgress
	result, err := (Downloader{VerifySigner: PlatformSignatureVerifier{}, Progress: func(progress DownloadProgress) { lastProgress = progress }}).Download(ctx, release, t.TempDir())
	if err != nil {
		t.Fatalf("live Helper download failed: %v (%s), progress=%+v", err, CodeOf(err), lastProgress)
	}
	if result.Bytes <= 300*1024*1024 || result.SHA256 != release.SHA256 || lastProgress.Phase != "complete" || lastProgress.Bytes != result.Bytes {
		t.Fatalf("unexpected live Helper result=%+v progress=%+v", result, lastProgress)
	}
}
