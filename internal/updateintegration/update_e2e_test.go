package updateintegration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meow-launcher/internal/installerbundle"
	"meow-launcher/internal/productinstaller"
	"meow-launcher/internal/updatecontract"
	"meow-launcher/internal/updateinstall"
)

func TestHTTPSManifestDownloadAndInstallBundleEndToEnd(t *testing.T) {
	root := t.TempDir()
	runner := filepath.Join(root, "runner.exe")
	payloadRoot := filepath.Join(root, "payload")
	installer := filepath.Join(root, "dev-helper-installer.exe")
	writeFile(t, runner, []byte("MZ test runner"))
	writeFile(t, filepath.Join(payloadRoot, "Dev Helper.exe"), []byte("complete helper application"))
	if _, err := installerbundle.Build(installerbundle.BuildOptions{
		RunnerPath: runner, PayloadRoot: payloadRoot, OutputPath: installer, ProductID: "dev-helper", Version: "0.3.1-preview.1", ExecutableRelativePath: "Dev Helper.exe",
	}); err != nil {
		t.Fatal(err)
	}
	installerBytes, err := os.ReadFile(installer)
	if err != nil {
		t.Fatal(err)
	}
	var manifestBytes []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/manifest-v1.json":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write(manifestBytes)
		case "/dev-helper-installer.exe":
			response.Header().Set("Content-Type", "application/octet-stream")
			_, _ = response.Write(installerBytes)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	manifest := updatecontract.Manifest{
		SchemaVersion: updatecontract.SchemaVersion,
		ManifestURL:   server.URL + "/manifest-v1.json",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Products: []updatecontract.ProductRelease{{
			ProductID: "dev-helper", Channel: "preview", Version: "0.3.1-preview.1", PublishedAt: time.Now().UTC().Format(time.RFC3339),
			ReleaseNotes: "TLS integration test", DownloadURL: server.URL + "/dev-helper-installer.exe", InstallerType: "meow-bundle", AccessPolicy: "public",
			InstallScope: "per-user", InstallDirectoryName: "Dev Helper", ExecutableRelativePath: "Dev Helper.exe",
			SHA256: fmt.Sprintf("%x", sha256.Sum256(installerBytes)), Signature: updatecontract.Signature{Method: "authenticode", Required: false},
			MinimumLauncherVersion: "0.6.6", MinimumSupportedVersion: "0.3.0-p1", DataMigrationVersion: "none",
			Rollback: updatecontract.Rollback{Strategy: "bundle-transaction", PreviousVersionSupported: true, BackupRequired: true},
		}},
	}
	manifestBytes, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	previousDefaultClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = previousDefaultClient })
	check := updatecontract.Check(context.Background(), server.URL+"/manifest-v1.json", "dev-helper", "0.3.0-p1", "preview")
	if check.Status != updatecontract.StatusUpdateAvailable || check.Release == nil {
		t.Fatalf("check=%+v", check)
	}
	download, err := (updateinstall.Downloader{HTTPClient: server.Client()}).Download(context.Background(), *check.Release, filepath.Join(root, "downloads"))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "installed", "Dev Helper", "Dev Helper.exe")
	receiptPath := filepath.Join(root, "receipts", "tx.json")
	receipt, err := productinstaller.Install(context.Background(), download.Path, productinstaller.Invocation{
		ProductID: "dev-helper", Version: check.Release.Version, TransactionID: "tx-tls-e2e", ReceiptPath: receiptPath, TargetExecutable: target,
	}, productinstaller.Options{})
	if err != nil || receipt.Status != "installed" {
		t.Fatalf("receipt=%+v error=%v", receipt, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "complete helper application" {
		t.Fatalf("installed=%q error=%v", got, err)
	}
}

func writeFile(t *testing.T, filename string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o700); err != nil {
		t.Fatal(err)
	}
}
