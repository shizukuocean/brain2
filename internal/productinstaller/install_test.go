package productinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"meow-launcher/internal/desktopshortcut"
	"meow-launcher/internal/installerbundle"
	"meow-launcher/internal/oneshot"
)

func TestMain(m *testing.M) {
	if marker := os.Getenv("MEOW_PRODUCT_RESTART_PROBE"); marker != "" {
		if err := os.WriteFile(marker, []byte("restarted"), 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestInstallAppliesCompleteBundleWritesReceiptAndRestarts(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "dev-helper", "1.0.1", "Dev Helper.exe", false, map[string]string{
		"Dev Helper.exe":     "new executable",
		"resources/app.asar": "new application",
	})
	targetRoot := filepath.Join(root, "installed", "Dev Helper")
	target := filepath.Join(targetRoot, "Dev Helper.exe")
	writeFile(t, target, "old executable")
	receiptPath := filepath.Join(root, "receipts", "tx.json")
	restarted := false
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "dev-helper", Version: "1.0.1", TransactionID: "tx-install", ReceiptPath: receiptPath,
		ReturnAction: "restart", TargetExecutable: target,
	}, Options{SkipDesktopShortcut: true, Apply: oneshot.ApplyOptions{Restart: func(executable string, _ []string) error {
		restarted = executable == target
		return nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "installed" || !restarted || readFile(t, target) != "new executable" || readFile(t, filepath.Join(targetRoot, "resources", "app.asar")) != "new application" {
		t.Fatalf("receipt=%+v restarted=%v", receipt, restarted)
	}
	var diskReceipt oneshot.Receipt
	if err := json.Unmarshal([]byte(readFile(t, receiptPath)), &diskReceipt); err != nil || diskReceipt.TransactionID != "tx-install" {
		t.Fatalf("disk receipt=%+v error=%v", diskReceipt, err)
	}
}

func TestOrdinaryProductUpdateStartsTheReplacedExecutable(t *testing.T) {
	root := t.TempDir()
	currentTestExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	payloadExecutable := filepath.Join(root, "payload", "Dev Helper.exe")
	copyFile(t, currentTestExecutable, payloadExecutable)
	runner := filepath.Join(root, "runner.exe")
	writeFile(t, runner, "MZ runner")
	installer := filepath.Join(root, "dev-helper-installer.exe")
	if _, err := installerbundle.Build(installerbundle.BuildOptions{
		RunnerPath: runner, PayloadRoot: filepath.Dir(payloadExecutable), OutputPath: installer,
		ProductID: "dev-helper", Version: "1.0.2", ExecutableRelativePath: "Dev Helper.exe",
	}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "installed", "Dev Helper.exe")
	writeFile(t, target, "old executable")
	marker := filepath.Join(root, "restart-marker.txt")
	t.Setenv("MEOW_PRODUCT_RESTART_PROBE", marker)
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "dev-helper", Version: "1.0.2", TransactionID: "tx-real-restart",
		ReceiptPath: filepath.Join(root, "receipt.json"), ReturnAction: "restart", TargetExecutable: target,
	}, Options{SkipDesktopShortcut: true})
	if err != nil || receipt.Status != "installed" || !receipt.Restarted {
		t.Fatalf("receipt=%+v error=%v", receipt, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if data, readErr := os.ReadFile(marker); readErr == nil && string(data) == "restarted" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("updated ordinary product did not start and write its restart marker")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLauncherSelfUpdateWaitsForExactPIDThenRestarts(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "meow-launcher", "2.0.0", "Meow Launcher.exe", true, map[string]string{"Meow Launcher.exe": "new launcher"})
	target := filepath.Join(root, "installed", "Meow Launcher.exe")
	writeFile(t, target, "old launcher")
	waited := 0
	restarted := false
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "meow-launcher", Version: "2.0.0", TransactionID: "tx-self", ReceiptPath: filepath.Join(root, "receipt.json"),
		WaitPID: 4321, ReturnAction: "restart", TargetExecutable: target, RestartLauncher: true,
	}, Options{SkipDesktopShortcut: true, Apply: oneshot.ApplyOptions{
		WaitForPID: func(_ context.Context, pid int) error { waited = pid; return nil },
		Restart:    func(executable string, _ []string) error { restarted = executable == target; return nil },
	}})
	if err != nil || receipt.Status != "installed" || waited != 4321 || !restarted || readFile(t, target) != "new launcher" {
		t.Fatalf("receipt=%+v error=%v waited=%d restarted=%v", receipt, err, waited, restarted)
	}
}

func TestInitialLauncherInstallUsesSelectedDirectoryThenRestarts(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "meow-launcher", "2.0.0", "Meow Launcher.exe", true, map[string]string{
		"Meow Launcher.exe": "new launcher", "runtime.dll": "launcher runtime",
	})
	installRoot := filepath.Join(root, "Meow System", "Launcher")
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(installRoot, "Meow Launcher.exe")
	restarted := false
	receipt, err := InstallInitialLauncher(context.Background(), installer, installRoot, Options{
		ReceiptRoot: filepath.Join(root, "receipts"), SkipDesktopShortcut: true,
		Apply: oneshot.ApplyOptions{Restart: func(executable string, _ []string) error {
			restarted = executable == target
			return nil
		}},
	})
	if err != nil || receipt.Status != "installed" || !receipt.Restarted || !restarted {
		t.Fatalf("receipt=%+v error=%v restarted=%v", receipt, err, restarted)
	}
	if readFile(t, target) != "new launcher" || readFile(t, filepath.Join(installRoot, "runtime.dll")) != "launcher runtime" {
		t.Fatal("initial Launcher install did not use the selected directory")
	}
	if matches, err := filepath.Glob(filepath.Join(root, "receipts", "setup-*.json")); err != nil || len(matches) != 1 {
		t.Fatalf("initial install receipt matches=%v error=%v", matches, err)
	}
}

func TestInstallCreatesDesktopShortcutBeforeRestart(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "dev-helper", "1.0.3", "Dev Helper.exe", false, map[string]string{"Dev Helper.exe": "new helper"})
	target := filepath.Join(root, "Meow System", "Helper", "Dev Helper.exe")
	writeFile(t, target, "old helper")
	shortcutCreated := false
	restartedAfterShortcut := false
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "dev-helper", Version: "1.0.3", TransactionID: "tx-shortcut",
		ReceiptPath: filepath.Join(root, "receipt-shortcut.json"), ReturnAction: "restart", TargetExecutable: target,
	}, Options{
		ShortcutWriter: func(request desktopshortcut.Request) (string, error) {
			shortcutCreated = request.TargetExecutable == target && request.Description == "打开 Dev Helper"
			return filepath.Join(root, "Desktop", "Dev Helper.lnk"), nil
		},
		Apply: oneshot.ApplyOptions{Restart: func(string, []string) error {
			restartedAfterShortcut = shortcutCreated
			return nil
		}},
	})
	if err != nil || receipt.Status != "installed" || !shortcutCreated || !restartedAfterShortcut {
		t.Fatalf("receipt=%+v error=%v shortcut=%v restart_after_shortcut=%v", receipt, err, shortcutCreated, restartedAfterShortcut)
	}
}

func TestInstallRollsBackWhenDesktopShortcutCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "brain2-manager", "1.0.1", "Brain2 Manager.exe", false, map[string]string{"Brain2 Manager.exe": "new manager"})
	target := filepath.Join(root, "Meow System", "Brain2 Manager", "Brain2 Manager.exe")
	writeFile(t, target, "old manager")
	forced := errors.New("desktop unavailable")
	restarted := false
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "brain2-manager", Version: "1.0.1", TransactionID: "tx-shortcut-failure",
		ReceiptPath: filepath.Join(root, "receipt-shortcut-failure.json"), ReturnAction: "restart", TargetExecutable: target,
	}, Options{
		ShortcutWriter: func(desktopshortcut.Request) (string, error) { return "", forced },
		Apply:          oneshot.ApplyOptions{Restart: func(string, []string) error { restarted = true; return nil }},
	})
	if !errors.Is(err, forced) || receipt.Status != "failed" || restarted {
		t.Fatalf("receipt=%+v error=%v restarted=%v", receipt, err, restarted)
	}
	if readFile(t, target) != "old manager" {
		t.Fatal("application was not restored after shortcut creation failed")
	}
}

func TestInitialLauncherInstallCannotReplaceExistingLauncher(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "meow-launcher", "2.0.0", "Meow Launcher.exe", true, map[string]string{"Meow Launcher.exe": "new launcher"})
	installRoot := filepath.Join(root, "Meow System", "Launcher")
	target := filepath.Join(installRoot, "Meow Launcher.exe")
	writeFile(t, target, "existing launcher")
	if _, err := InstallInitialLauncher(context.Background(), installer, installRoot, Options{ReceiptRoot: filepath.Join(root, "receipts")}); err == nil {
		t.Fatal("expected initial setup to refuse replacing an existing Launcher")
	}
	if readFile(t, target) != "existing launcher" {
		t.Fatal("initial setup changed the existing Launcher")
	}
}

func TestOnlyLauncherOneShotBundleCanRunInteractiveInitialInstall(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "dev-helper", "1.0.0", "Dev Helper.exe", false, map[string]string{"Dev Helper.exe": "helper"})
	installRoot := filepath.Join(root, "selected")
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallInitialLauncher(context.Background(), installer, installRoot, Options{ReceiptRoot: filepath.Join(root, "receipts"), SkipDesktopShortcut: true}); err == nil {
		t.Fatal("expected ordinary product bundle to reject interactive Launcher setup")
	}
}

func TestExistingLegacyExecutableNameIsUpdatedInPlace(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "meow-launcher", "2.0.0", "Meow Launcher.exe", true, map[string]string{"Meow Launcher.exe": "new launcher"})
	target := filepath.Join(root, "installed", "Meow Launcher v0.6.6 WIP.exe")
	writeFile(t, target, "old launcher")
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "meow-launcher", Version: "2.0.0", TransactionID: "tx-legacy-name", ReceiptPath: filepath.Join(root, "receipt-legacy.json"),
		WaitPID: 100, ReturnAction: "restart", TargetExecutable: target, RestartLauncher: true,
	}, Options{SkipDesktopShortcut: true, Apply: oneshot.ApplyOptions{WaitForPID: func(context.Context, int) error { return nil }, Restart: func(string, []string) error { return nil }}})
	if err != nil || receipt.Status != "installed" || readFile(t, target) != "new launcher" {
		t.Fatalf("receipt=%+v error=%v target=%q", receipt, err, readFile(t, target))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(target), "Meow Launcher.exe")); !os.IsNotExist(err) {
		t.Fatalf("update created a second canonical executable: %v", err)
	}
}

func TestInvocationMismatchCannotChangeExistingProduct(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "brain2-manager", "1.0.0", "Brain2 Manager.exe", false, map[string]string{"Brain2 Manager.exe": "new"})
	target := filepath.Join(root, "installed", "Brain2 Manager.exe")
	writeFile(t, target, "old")
	_, err := Install(context.Background(), installer, Invocation{
		ProductID: "dev-helper", Version: "1.0.0", TransactionID: "tx", ReceiptPath: filepath.Join(root, "receipt.json"), TargetExecutable: target,
	}, Options{SkipDesktopShortcut: true})
	if err == nil || readFile(t, target) != "old" {
		t.Fatalf("error=%v target=%q", err, readFile(t, target))
	}
}

func TestUpdateRemovesOnlyFilesOwnedByPreviousPackage(t *testing.T) {
	root := t.TempDir()
	first := buildTestBundle(t, root, "dev-helper", "1.0.0", "Dev Helper.exe", false, map[string]string{
		"Dev Helper.exe": "version one", "old-runtime.dll": "owned old runtime",
	})
	target := filepath.Join(root, "installed", "Dev Helper.exe")
	if _, err := Install(context.Background(), first, Invocation{
		ProductID: "dev-helper", Version: "1.0.0", TransactionID: "tx-first", ReceiptPath: filepath.Join(root, "first.json"), TargetExecutable: target,
	}, Options{SkipDesktopShortcut: true}); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(filepath.Dir(target), "user-notes.txt")
	writeFile(t, userFile, "must remain")
	second := buildTestBundle(t, root, "dev-helper", "1.1.0", "Dev Helper.exe", false, map[string]string{"Dev Helper.exe": "version two"})
	if _, err := Install(context.Background(), second, Invocation{
		ProductID: "dev-helper", Version: "1.1.0", TransactionID: "tx-second", ReceiptPath: filepath.Join(root, "second.json"), TargetExecutable: target,
	}, Options{SkipDesktopShortcut: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(target), "old-runtime.dll")); !os.IsNotExist(err) {
		t.Fatalf("old owned file remains: %v", err)
	}
	if readFile(t, userFile) != "must remain" || readFile(t, target) != "version two" {
		t.Fatal("update changed an unowned user file or failed to replace the app")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(target), installerbundle.InstalledFilesManifestName)); err != nil {
		t.Fatal("installed file ownership manifest missing")
	}
	owned, err := InstalledFilesForExecutable(target, "dev-helper")
	if err != nil || len(owned) != 2 {
		t.Fatalf("owned=%v error=%v", owned, err)
	}
}

func TestPreflightFailureWritesAReconciliableFailureReceipt(t *testing.T) {
	root := t.TempDir()
	installer := buildTestBundle(t, root, "brain2-manager", "1.0.0", "Brain2 Manager.exe", false, map[string]string{"Brain2 Manager.exe": "manager"})
	receiptPath := filepath.Join(root, "receipt.json")
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "dev-helper", Version: "1.0.0", TransactionID: "tx-mismatch", ReceiptPath: receiptPath, TargetExecutable: filepath.Join(root, "Brain2 Manager.exe"),
	}, Options{SkipDesktopShortcut: true})
	if err == nil || receipt.Status != "failed" {
		t.Fatalf("receipt=%+v error=%v", receipt, err)
	}
	var diskReceipt oneshot.Receipt
	if jsonErr := json.Unmarshal([]byte(readFile(t, receiptPath)), &diskReceipt); jsonErr != nil || diskReceipt.Status != "failed" || diskReceipt.TransactionID != "tx-mismatch" {
		t.Fatalf("disk receipt=%+v error=%v", diskReceipt, jsonErr)
	}
}

func TestLegacyOwnedLauncherSelfUpdateInstallsCanonicalTargetAndKeepsFallback(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	t.Setenv("MEOW_DESKTOP_SHORTCUT_TEST_ROOT", filepath.Join(machineRoot, "desktop"))
	installer := buildTestBundle(t, machineRoot, "meow-launcher", "2.0.0", "Meow Launcher.exe", true, map[string]string{
		"Meow Launcher.exe": "new canonical launcher",
	})
	legacyRoot := filepath.Join(machineRoot, "legacy-launcher")
	legacyExecutable := filepath.Join(legacyRoot, "Meow Launcher.exe")
	writeFile(t, legacyExecutable, "old launcher fallback")
	writeOwnershipManifest(t, legacyRoot, "meow-launcher", "1.9.0", []string{"Meow Launcher.exe"})
	canonicalExecutable := filepath.Join(machineRoot, "Meow System", "Launcher", "Meow Launcher.exe")
	shortcutTarget := ""
	restarted := ""
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "meow-launcher", Version: "2.0.0", TransactionID: "tx-legacy-migration",
		ReceiptPath: filepath.Join(machineRoot, "receipt.json"), WaitPID: 4321, ReturnAction: "restart",
		TargetExecutable: legacyExecutable, RestartLauncher: true,
	}, Options{
		ShortcutWriter: func(request desktopshortcut.Request) (string, error) {
			shortcutTarget = request.TargetExecutable
			return filepath.Join(machineRoot, "desktop", "Meow Launcher.lnk"), nil
		},
		Apply: oneshot.ApplyOptions{
			WaitForPID: func(context.Context, int) error { return nil },
			Restart: func(executable string, _ []string) error {
				restarted = executable
				return nil
			},
		},
	})
	if err != nil || receipt.Status != "installed" || !receipt.Restarted {
		t.Fatalf("receipt=%+v error=%v", receipt, err)
	}
	if receipt.ExecutablePath != canonicalExecutable || restarted != canonicalExecutable || shortcutTarget != canonicalExecutable {
		t.Fatalf("receipt=%q restarted=%q shortcut=%q want=%q", receipt.ExecutablePath, restarted, shortcutTarget, canonicalExecutable)
	}
	if readFile(t, canonicalExecutable) != "new canonical launcher" || readFile(t, legacyExecutable) != "old launcher fallback" {
		t.Fatal("canonical install failed or legacy fallback changed before restart reconciliation")
	}
}

func TestLegacyLauncherMigrationRefusesNonemptyCanonicalDirectory(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	t.Setenv("MEOW_DESKTOP_SHORTCUT_TEST_ROOT", filepath.Join(machineRoot, "desktop"))
	installer := buildTestBundle(t, machineRoot, "meow-launcher", "2.0.0", "Meow Launcher.exe", true, map[string]string{
		"Meow Launcher.exe": "new launcher",
	})
	legacyRoot := filepath.Join(machineRoot, "legacy-launcher")
	legacyExecutable := filepath.Join(legacyRoot, "Meow Launcher.exe")
	writeFile(t, legacyExecutable, "old launcher")
	writeOwnershipManifest(t, legacyRoot, "meow-launcher", "1.9.0", []string{"Meow Launcher.exe"})
	canonicalRoot := filepath.Join(machineRoot, "Meow System", "Launcher")
	writeFile(t, filepath.Join(canonicalRoot, "unowned.txt"), "do not overwrite")
	receipt, err := Install(context.Background(), installer, Invocation{
		ProductID: "meow-launcher", Version: "2.0.0", TransactionID: "tx-collision",
		ReceiptPath: filepath.Join(machineRoot, "collision-receipt.json"), WaitPID: 4321, ReturnAction: "restart",
		TargetExecutable: legacyExecutable, RestartLauncher: true,
	}, Options{SkipDesktopShortcut: true})
	if err == nil || receipt.Status != "failed" || readFile(t, legacyExecutable) != "old launcher" || readFile(t, filepath.Join(canonicalRoot, "unowned.txt")) != "do not overwrite" {
		t.Fatalf("receipt=%+v error=%v", receipt, err)
	}
}

func buildTestBundle(t *testing.T, root, productID, version, entrypoint string, selfUpdate bool, files map[string]string) string {
	t.Helper()
	runner := filepath.Join(root, productID+"-runner.exe")
	payload := filepath.Join(root, productID+"-"+version+"-payload")
	output := filepath.Join(root, productID+"-"+version+"-installer.exe")
	writeFile(t, runner, "MZ runner")
	for name, content := range files {
		writeFile(t, filepath.Join(payload, filepath.FromSlash(name)), content)
	}
	if _, err := installerbundle.Build(installerbundle.BuildOptions{
		RunnerPath: runner, PayloadRoot: payload, OutputPath: output, ProductID: productID, Version: version, ExecutableRelativePath: entrypoint, SelfUpdate: selfUpdate,
	}); err != nil {
		t.Fatal(err)
	}
	return output
}

func writeFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeOwnershipManifest(t *testing.T, root, productID, version string, files []string) {
	t.Helper()
	data, err := json.Marshal(installedFilesManifest{
		SchemaVersion: installedFilesSchemaVersion,
		ProductID:     productID,
		Version:       version,
		Files:         files,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, installerbundle.InstalledFilesManifestName), string(data))
}

func readFile(t *testing.T, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func copyFile(t *testing.T, source, destination string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}
