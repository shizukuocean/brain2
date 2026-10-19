package updateinstall

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestExecutorRechecksPayloadImmediatelyBeforeStarting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installer.exe")
	payload := []byte("verified")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	release := validRelease("https://downloads.example.invalid/setup.exe", payload)
	started := false
	executor := Executor{VerifySigner: &recordingVerifier{}, Start: func(_ context.Context, _ string, _ []string) (int, error) {
		started = true
		return 123, nil
	}}
	if err := os.WriteFile(path, []byte("changed after download"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := executor.Execute(context.Background(), ExecuteRequest{Release: release, InstallerPath: path, TransactionID: "tx", ReceiptPath: filepath.Join(t.TempDir(), "receipt.json")})
	if CodeOf(err) != "HASH_MISMATCH" || started {
		t.Fatalf("error=%v started=%v", err, started)
	}
}

func TestStartInstallerOutlivesCancelledLauncherRPC(t *testing.T) {
	markerPrefix := "meow-installer-child-marker="
	for _, argument := range os.Args {
		if strings.HasPrefix(argument, markerPrefix) {
			time.Sleep(250 * time.Millisecond)
			_ = os.WriteFile(strings.TrimPrefix(argument, markerPrefix), []byte("installer survived"), 0o600)
			os.Exit(0)
		}
	}
	marker := filepath.Join(t.TempDir(), "child-finished.txt")
	ctx, cancel := context.WithCancel(context.Background())
	_, err := startInstaller(ctx, os.Args[0], []string{"-test.run=^TestStartInstallerOutlivesCancelledLauncherRPC$", "--", markerPrefix + marker})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, readErr := os.ReadFile(marker); readErr == nil && string(data) == "installer survived" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("installer process was tied to the cancelled Launcher RPC")
}

func TestExecutorStartsProductInstallerWithExplicitRestartTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installer.exe")
	target := filepath.Join(t.TempDir(), "Dev Helper.exe")
	payload := []byte("verified")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	release := validRelease("https://downloads.example.invalid/setup.exe", payload)
	var gotArgs []string
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	result, err := (Executor{VerifySigner: &recordingVerifier{}, Start: func(_ context.Context, gotPath string, args []string) (int, error) {
		if gotPath != path {
			t.Fatalf("path=%q", gotPath)
		}
		gotArgs = append([]string(nil), args...)
		return 321, nil
	}}).Execute(context.Background(), ExecuteRequest{Release: release, InstallerPath: path, TransactionID: "tx-product", ReceiptPath: receiptPath, ReturnAction: "restart", TargetExecutable: target})
	if err != nil || result.Status != "installer_started" || result.PID != 321 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	want := []string{"--meow-product", "dev-helper", "--meow-version", "1.2.3", "--meow-transaction", "tx-product", "--meow-receipt", receiptPath, "--meow-return", "restart", "--meow-target-executable", target}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("args=%v want=%v", gotArgs, want)
	}
}

func TestExecutorStartsLauncherOneShotThenCallerCanExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launcher-one-shot.exe")
	payload := []byte("verified")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	release := validRelease("https://downloads.example.invalid/setup.exe", payload)
	release.ProductID = "meow-launcher"
	release.InstallerType = "nsis-one-shot"
	var gotArgs []string
	result, err := (Executor{VerifySigner: &recordingVerifier{}, Start: func(_ context.Context, _ string, args []string) (int, error) {
		gotArgs = append([]string(nil), args...)
		return 654, nil
	}}).Execute(context.Background(), ExecuteRequest{Release: release, InstallerPath: path, TransactionID: "tx-self", ReceiptPath: filepath.Join(t.TempDir(), "receipt.json"), LauncherPID: 999, ReturnAction: "restart"})
	if err != nil || result.Status != "self_update_installer_started" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	wantSuffix := []string{"--meow-wait-pid", "999", "--meow-restart-launcher"}
	if len(gotArgs) < len(wantSuffix) || !reflect.DeepEqual(gotArgs[len(gotArgs)-len(wantSuffix):], wantSuffix) {
		t.Fatalf("args=%v", gotArgs)
	}
}

func TestExecutorAcceptsSelfContainedMeowBundleTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launcher-bundle.exe")
	payload := []byte("verified")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	release := validRelease("https://downloads.example.invalid/setup.exe", payload)
	release.ProductID = "meow-launcher"
	release.InstallerType = "meow-bundle-one-shot"
	result, err := (Executor{VerifySigner: &recordingVerifier{}, Start: func(_ context.Context, _ string, _ []string) (int, error) {
		return 765, nil
	}}).Execute(context.Background(), ExecuteRequest{
		Release: release, InstallerPath: path, TransactionID: "tx-bundle", ReceiptPath: filepath.Join(t.TempDir(), "receipt.json"),
		LauncherPID: 999, ReturnAction: "restart", TargetExecutable: filepath.Join(t.TempDir(), "Meow Launcher.exe"),
	})
	if err != nil || result.Status != "self_update_installer_started" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}
