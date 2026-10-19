package updatecontract

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckLocalManifestFindsPublicUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	data := strings.ReplaceAll(testManifest, "CURRENT_VERSION", "0.6.7")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Check(context.Background(), path, "brain2-manager", "0.6.6", "preview")
	if result.Status != StatusUpdateAvailable || result.Release == nil || result.Release.Version != "0.6.7" {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestCheckSeparatesMissingSourceFromNoUpdate(t *testing.T) {
	result := Check(context.Background(), "", "dev-helper", "0.3.0-p1", "preview")
	if result.Status != StatusSourceNotConfigured || result.ErrorCode != "UPDATE_SOURCE_NOT_CONFIGURED" {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestManifestRejectsInsecureDownload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	data := strings.ReplaceAll(strings.ReplaceAll(testManifest, "CURRENT_VERSION", "0.6.7"), "https://downloads.example.invalid", "http://downloads.example.invalid")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Check(context.Background(), path, "brain2-manager", "0.6.6", "preview")
	if result.Status != StatusManifestInvalid || result.ErrorCode != "UPDATE_MANIFEST_INVALID" {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestManifestRejectsMalformedVersionsAndEmbeddedDownloadCredentials(t *testing.T) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(strings.ReplaceAll(testManifest, "CURRENT_VERSION", "0.6.7")), &manifest); err != nil {
		t.Fatal(err)
	}
	badVersion := manifest.Products[0]
	badVersion.Version = "preview-latest"
	if err := badVersion.Validate(); err == nil || !strings.Contains(err.Error(), "version 版本格式无效") {
		t.Fatalf("malformed version result: %v", err)
	}
	credentialURL := manifest.Products[0]
	credentialURL.DownloadURL = "https://token@downloads.example.invalid/brain2-manager.exe"
	if err := credentialURL.Validate(); err == nil || !strings.Contains(err.Error(), "不能内嵌账号或令牌") {
		t.Fatalf("embedded credential result: %v", err)
	}
}

func TestManifestRequiresPermanentHTTPSManifestURL(t *testing.T) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(strings.ReplaceAll(testManifest, "CURRENT_VERSION", "0.6.7")), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ManifestURL = "http://downloads.example.invalid/channels/preview/manifest-v1.json"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "manifest_url 必须使用 HTTPS") {
		t.Fatalf("insecure manifest URL result: %v", err)
	}
	manifest.ManifestURL = "https://token@downloads.example.invalid/channels/preview/manifest-v1.json"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "manifest_url 不能内嵌账号或令牌") {
		t.Fatalf("embedded manifest credential result: %v", err)
	}
}

func TestStableChannelRejectsUnsignedInstaller(t *testing.T) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(strings.ReplaceAll(testManifest, "CURRENT_VERSION", "0.6.7")), &manifest); err != nil {
		t.Fatal(err)
	}
	release := manifest.Products[0]
	release.Channel = "stable"
	release.Signature.Required = false
	release.Signature.Publisher = ""
	if err := release.Validate(); err == nil {
		t.Fatal("stable release accepted an unsigned installer")
	}
}

func TestLauncherRequiresOneShotInstallerWhileProductsRejectIt(t *testing.T) {
	var manifest Manifest
	if err := json.Unmarshal([]byte(strings.ReplaceAll(testManifest, "CURRENT_VERSION", "0.6.7")), &manifest); err != nil {
		t.Fatal(err)
	}
	manager := manifest.Products[0]
	manager.InstallerType = "meow-bundle-one-shot"
	if err := manager.Validate(); err == nil {
		t.Fatal("ordinary product accepted a Launcher self-update package")
	}
	launcher := manager
	launcher.ProductID = "meow-launcher"
	launcher.InstallerType = "meow-bundle"
	if err := launcher.Validate(); err == nil {
		t.Fatal("Launcher accepted a non-one-shot package")
	}
	launcher.InstallerType = "meow-bundle-one-shot"
	if err := launcher.Validate(); err != nil {
		t.Fatalf("valid Launcher one-shot package rejected: %v", err)
	}
}

func TestCompareVersionsSupportsCurrentPreviewLabels(t *testing.T) {
	for _, sample := range []struct {
		left, right string
		want        int
	}{
		{"0.6.7", "0.6.6 WIP", 1}, {"0.3.1", "0.3.0-p1", 1}, {"0.6.6", "0.6.6 WIP", 1}, {"0.6.6", "0.6.6", 0},
		{"0.6.7-preview.10", "0.6.7-preview.2", 1}, {"0.6.7-preview.2", "0.6.7-preview.10", -1},
		{"0.3.0-p10", "0.3.0-p2", 1}, {"0.6.7-preview", "0.6.7-preview.1", -1}, {"0.6.7", "0.6.7-preview.99", 1},
	} {
		got := CompareVersions(sample.left, sample.right)
		if got != sample.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", sample.left, sample.right, got, sample.want)
		}
	}
}

func TestNetworkFailureAndHashMismatchRemainDistinct(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Check(ctx, "https://127.0.0.1:1/manifest.json", "dev-helper", "0.3.0-p1", "preview")
	if result.Status != StatusCheckFailed || result.ErrorCode != "UPDATE_CHECK_FAILED" {
		t.Fatalf("unexpected network result %#v", result)
	}
	path := filepath.Join(t.TempDir(), "download.exe")
	if err := os.WriteFile(path, []byte("download"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFileSHA256(path, strings.Repeat("0", 64)); CodeOf(err) != "HASH_MISMATCH" {
		t.Fatalf("expected HASH_MISMATCH, got %v", err)
	}
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("download")))
	if err := VerifyFileSHA256(path, expected); err != nil {
		t.Fatal(err)
	}
}

const testManifest = `{
  "schema_version": "meow-update-manifest/v1",
  "manifest_url": "https://downloads.example.invalid/channels/preview/manifest-v1.json",
  "generated_at": "2026-08-11T00:00:00Z",
  "products": [{
    "product_id": "brain2-manager",
    "channel": "preview",
    "version": "CURRENT_VERSION",
    "published_at": "2026-08-11T00:00:00Z",
    "release_notes": "本地测试更新。",
    "download_url": "https://downloads.example.invalid/brain2-manager.exe",
    "installer_type": "nsis",
    "access_policy": "public",
    "install_scope": "per-user",
    "install_directory_name": "Brain2 Manager",
    "executable_relative_path": "Brain2 Manager.exe",
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "signature": {"method": "authenticode", "required": true, "publisher": "Meow Test Publisher"},
    "minimum_launcher_version": "0.6.6",
    "minimum_supported_version": "0.6.0",
    "mandatory": false,
    "data_migration_version": "none",
    "rollback": {"strategy": "replace-and-restore", "previous_version_supported": true, "backup_required": true}
  }]
}`
