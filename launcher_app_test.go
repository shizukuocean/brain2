//go:build launcher

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meow-launcher/internal/installlayout"
	"meow-launcher/internal/settings"
	"meow-launcher/internal/updatecontract"
	"meow-launcher/internal/updatehistory"
	"meow-launcher/internal/updateinstall"
)

func TestLauncherSurfaceReportsCompleteCatalogAndSeparateManager(t *testing.T) {
	temp := t.TempDir()
	managerPath := filepath.Join(temp, brain2ManagerFilename)
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := NewLauncherApp().GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Authenticated || state.DownloadService != "ready" {
		t.Fatalf("expected honest local-preview account state, got %#v", state)
	}
	if state.AccountStatus != "公开 Preview · 当前无需登录" {
		t.Fatalf("public Preview should not claim sign-in is required, got %q", state.AccountStatus)
	}
	seen := map[string]LauncherProductState{}
	for _, product := range state.Products {
		seen[product.ID] = product
	}
	for _, productID := range []string{"brain2-manager", "meow-launcher", "meowdesk", "meow-protector", "dev-helper", "meow-viewer"} {
		if _, ok := seen[productID]; !ok {
			t.Fatalf("expected unified product catalog to include %q, got %#v", productID, state.Products)
		}
	}
	manager := seen["brain2-manager"]
	if !manager.Installed || !manager.CanLaunch || manager.ExecutablePath != managerPath {
		t.Fatalf("expected separate manager at %q, got %#v", managerPath, manager)
	}
	if manager.CurrentVersion != "0.6.6 WIP" || manager.UpdateStatus != "not_checked" || !manager.CanUninstall {
		t.Fatalf("expected installed product version and lifecycle state, got %#v", manager)
	}
	if seen["dev-helper"].CatalogVersion != "0.3.0-p1" || seen["meow-protector"].CatalogVersion != "0.1.0-gate-e" {
		t.Fatalf("expected product detail versions, got %#v", seen)
	}
}

func TestLauncherInfersAndPersistsMeowSystemRootFromItsInstalledPath(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_DATA_ROOT", dataRoot)
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", dataRoot)
	launcherPath := filepath.Join(t.TempDir(), installlayout.SystemDirectoryName, "Launcher", "Meow Launcher.exe")
	app := NewLauncherAppWithArgs(nil)
	app.currentExecutable = func() (string, error) { return launcherPath, nil }

	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := filepath.Dir(filepath.Dir(launcherPath))
	if state.MeowSystemStatus != "ready" || state.MeowSystemRoot != wantRoot {
		t.Fatalf("state root = %q (%s), want %q", state.MeowSystemRoot, state.MeowSystemStatus, wantRoot)
	}
	saved, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.MeowSystemRoot != wantRoot {
		t.Fatalf("persisted root = %q, want %q", saved.MeowSystemRoot, wantRoot)
	}
	for _, product := range state.Products {
		if product.ID == "dev-helper" {
			wantHelper := filepath.Join(wantRoot, "Helper")
			if product.InstallDirectory != wantHelper {
				t.Fatalf("helper target = %q, want %q", product.InstallDirectory, wantHelper)
			}
		}
	}
}

func TestLauncherFrontendKeepsWindowControlsVisibleAndPublicPreviewUngated(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("frontend", "launcher", "src", "App.vue"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, expected := range []string{
		`<svg viewBox="0 0 16 16" aria-hidden="true">`,
		`checkToInstall: '检查后安装'`,
		`accountTitle: '公开 Preview 无需登录'`,
		`await checkProductUpdate(product)`,
		`function installStatusLabel(product: LauncherProductState)`,
		`function formatDateTime(value?: string)`,
		`class="release-note"`,
		`downloadSlowHint: '正在连接并下载。若 45 秒没有收到任何数据，Launcher 会安全停止并允许重试。'`,
		`EventsOn('launcher-download-progress', receiveDownloadProgress)`,
		`EventsOn('launcher-update-request', receiveLauncherUpdateRequest)`,
		`async function receiveLauncherUpdateRequest(request: LauncherUpdateRequest)`,
		`const checked = await checkProductUpdate(requested)`,
		`await prepareProductUpdate(checked)`,
		`class="download-progress-track"`,
		`downloadProgress?.product_id === selectedProduct.id`,
		`class="download-failure"`,
		`class="download-ready"`,
		`updateActionArea.value?.scrollIntoView`,
		`'下载与校验完成'`,
		`formatBytes(downloadProgress?.bytes || 0)`,
		`async function installProduct(product: LauncherProductState)`,
		`downloadProgress.value = null`,
		`activeView.value = 'settings'`,
		`state.value = await backend.chooseMeowSystemRoot()`,
		`current = await backend.prepareProductUpdate(current.id)`,
		`current = await backend.executePreparedUpdate(current.id)`,
		`await waitForInstallReceipt(current.id, true)`,
		`class="primary install-product-action"`,
		`class="primary open-product-action"`,
		`` + "`打开 ${selectedProduct.name}`" + ``,
		`<button v-if="selectedProduct.installed" class="secondary"`,
		`class="primary install-final-action"`,
		`navProduct: '应用详细'`,
		`navUninstall: '应用列表'`,
		`navSettings: '设置'`,
		`v-for="product in state.products"`,
		`@click="openProductDetail(product, true)"`,
		`@click="uninstallCatalogProduct(product)"`,
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("Launcher frontend is missing %q", expected)
		}
	}
	for _, forbidden := range []string{`downloadAfterLogin: '登录后下载'`, `disabled>{{ state.authenticated ?`, `const selected = await chooseInstallDirectory(product)`, `使用推荐目录（推荐）`, `installDirectoryDraft`, `install-directory-editor`, `selectedProduct.can_launch || ['update_available', 'current'].includes(selectedProduct.update_status)`, `class="install-step-note"`} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("Launcher frontend still contains obsolete sign-in gate %q", forbidden)
		}
	}
	launcherData, err := os.ReadFile("launcher_app.go")
	if err != nil {
		t.Fatal(err)
	}
	launcherSource := string(launcherData)
	for _, fragment := range []string{
		`wailsruntime.WindowShow(ctx)`,
		`wailsruntime.WindowUnminimise(ctx)`,
		`bringLauncherWindowToFront()`,
	} {
		if !strings.Contains(launcherSource, fragment) {
			t.Fatalf("product-triggered update must visibly foreground Launcher; missing %q", fragment)
		}
	}
}

func TestPrepareProductUpdateRequiresFreshAuthorizationBeforeDownload(t *testing.T) {
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs(nil)
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &updatecontract.ProductRelease{ProductID: "brain2-manager", Version: "0.6.7"}}
	}
	app.authorizeInstall = func(updatecontract.ProductRelease) updateinstall.AuthorizationResult {
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationDenied, ErrorCode: "AUTH_DENIED", Message: "purchase required"}
	}
	downloaded := false
	app.downloadRelease = func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		downloaded = true
		return updateinstall.DownloadResult{}, nil
	}
	product, err := app.PrepareProductUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if downloaded || product.AuthorizationStatus != "denied" || product.InstallStatus != "authorization_required" || product.UpdateStatus != "update_available" {
		t.Fatalf("product=%+v downloaded=%v", product, downloaded)
	}
}

func TestPrepareProductUpdateRequiresLauncherUpgradeBeforeDownload(t *testing.T) {
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs(nil)
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &updatecontract.ProductRelease{
			ProductID: "brain2-manager", Version: "0.6.7", MinimumLauncherVersion: "0.6.7", AccessPolicy: "public",
		}}
	}
	downloaded := false
	app.downloadRelease = func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		downloaded = true
		return updateinstall.DownloadResult{}, nil
	}
	product, err := app.PrepareProductUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if downloaded || product.UpdateStatus != "launcher_update_required" || product.InstallStatus != "not_available" || product.ErrorCode != "MINIMUM_LAUNCHER_VERSION" {
		t.Fatalf("product=%+v downloaded=%v", product, downloaded)
	}
}

func TestPrepareProductUpdateDownloadsOnlyAfterAuthorization(t *testing.T) {
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs(nil)
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &updatecontract.ProductRelease{ProductID: "brain2-manager", Version: "0.6.7"}}
	}
	app.authorizeInstall = func(release updatecontract.ProductRelease) updateinstall.AuthorizationResult {
		if release.ProductID != "brain2-manager" || release.Version != "0.6.7" {
			t.Fatalf("unexpected authorization target %s %s", release.ProductID, release.Version)
		}
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationAuthorized, AuthorizationID: "fresh-grant", Message: "authorized"}
	}
	app.downloadRelease = func(release updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		return updateinstall.DownloadResult{Path: filepath.Join(t.TempDir(), "verified-installer.exe"), SignatureOkay: true}, nil
	}
	product, err := app.PrepareProductUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if product.InstallStatus != "ready_to_install" || product.DownloadedPath == "" || product.AuthorizationStatus != "authorized" {
		t.Fatalf("product=%+v", product)
	}
}

func TestExecutePreparedUpdateReauthorizesAndStartsExactProductInstaller(t *testing.T) {
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs([]string{"--update-product", "brain2-manager", "--return", "restart"})
	release := updatecontract.ProductRelease{ProductID: "brain2-manager", Version: "0.6.7", InstallerType: "nsis", ExecutableRelativePath: "Brain2 Manager.exe"}
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &release}
	}
	authorizationChecks := 0
	app.authorizeInstall = func(updatecontract.ProductRelease) updateinstall.AuthorizationResult {
		authorizationChecks++
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationAuthorized, AuthorizationID: "grant"}
	}
	installerPath := filepath.Join(t.TempDir(), "manager-installer.exe")
	app.downloadRelease = func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		return updateinstall.DownloadResult{Path: installerPath}, nil
	}
	var request updateinstall.ExecuteRequest
	closedTarget := ""
	closeCompleted := false
	app.closeProduct = func(_ context.Context, target string) error {
		closedTarget = target
		closeCompleted = true
		return nil
	}
	app.executeInstaller = func(got updateinstall.ExecuteRequest) (updateinstall.ExecuteResult, error) {
		if !closeCompleted {
			t.Fatal("installer started before the installed product exited")
		}
		request = got
		return updateinstall.ExecuteResult{Status: "installer_started", PID: 456}, nil
	}
	if _, err := app.PrepareProductUpdate("brain2-manager"); err != nil {
		t.Fatal(err)
	}
	product, err := app.ExecutePreparedUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	wantDirectory, layoutErr := installlayout.DirectoryFromSelection(managerPath, "brain2-manager")
	if layoutErr != nil {
		t.Fatal(layoutErr)
	}
	wantTarget := filepath.Join(wantDirectory, "Brain2 Manager.exe")
	if authorizationChecks != 2 || product.InstallStatus != "installer_started" || closedTarget != managerPath || request.InstallerPath != installerPath || request.TargetExecutable != wantTarget || request.ReturnAction != "restart" {
		t.Fatalf("checks=%d closed=%q product=%+v request=%+v", authorizationChecks, closedTarget, product, request)
	}
}

func TestExecutePreparedUpdateStopsBeforeReplacementWhenProductWillNotClose(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	managerPath := filepath.Join(machineRoot, brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs(nil)
	app.historyReady = true
	release := updatecontract.ProductRelease{ProductID: "brain2-manager", Version: "0.6.7", InstallerType: "nsis", ExecutableRelativePath: "Brain2 Manager.exe"}
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &release}
	}
	app.authorizeInstall = func(updatecontract.ProductRelease) updateinstall.AuthorizationResult {
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationAuthorized, AuthorizationID: "grant"}
	}
	app.downloadRelease = func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		return updateinstall.DownloadResult{Path: filepath.Join(machineRoot, "manager-installer.exe")}, nil
	}
	closeAttempts := 0
	app.closeProduct = func(context.Context, string) error {
		closeAttempts++
		if closeAttempts == 1 {
			return errors.New("still running")
		}
		return nil
	}
	installerStarted := false
	app.executeInstaller = func(updateinstall.ExecuteRequest) (updateinstall.ExecuteResult, error) {
		installerStarted = true
		return updateinstall.ExecuteResult{Status: "installer_started", PID: 901}, nil
	}
	if _, err := app.PrepareProductUpdate("brain2-manager"); err != nil {
		t.Fatal(err)
	}
	product, err := app.ExecutePreparedUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if installerStarted || product.InstallStatus != "product_still_running" || product.ErrorCode != "PRODUCT_STILL_RUNNING" {
		t.Fatalf("installerStarted=%v product=%+v", installerStarted, product)
	}
	record, ok := app.installRecords["brain2-manager"]
	if !ok || record.Status != "install_start_failed" || record.ErrorCode != "PRODUCT_STILL_RUNNING" {
		t.Fatalf("close failure was not recorded: %+v", record)
	}
	product, err = app.ExecutePreparedUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if !installerStarted || closeAttempts != 2 || product.InstallStatus != "installer_started" {
		t.Fatalf("retry did not reuse the verified download: attempts=%d started=%v product=%+v", closeAttempts, installerStarted, product)
	}
	record = app.installRecords["brain2-manager"]
	wantDirectory, layoutErr := installlayout.DirectoryFromSelection(managerPath, "brain2-manager")
	if layoutErr != nil {
		t.Fatal(layoutErr)
	}
	if record.PreviousExecutablePath != managerPath || record.ExecutablePath != filepath.Join(wantDirectory, "Brain2 Manager.exe") || record.MigrationStatus != "pending" {
		t.Fatalf("legacy Manager migration was not persisted: %+v", record)
	}
}

func TestOlderSuccessfulInstallRecordDoesNotHidePreparedUpdate(t *testing.T) {
	prepared := LauncherProductState{
		ID: "dev-helper", LatestVersion: "0.3.1-preview.3", UpdateStatus: "update_available",
		UpdateMessage: "new release available", InstallStatus: "ready_to_install",
		InstallMessage: "verified installer ready", DownloadedPath: `C:\cache\dev-helper-preview.3.exe`,
	}
	record := updatehistory.ProductRecord{
		ProductID: "dev-helper", InstalledVersion: "0.3.1-preview.2", Status: "installed",
		Message: "previous version installed", ExecutablePath: `D:\helper\Dev Helper.exe`,
	}

	product := applyInstallRecord(prepared, record)
	if product.CurrentVersion != "0.3.1-preview.2" || !product.Installed || product.ExecutablePath != record.ExecutablePath {
		t.Fatalf("record did not establish the real installed product: %+v", product)
	}
	if product.UpdateStatus != "update_available" || product.InstallStatus != "ready_to_install" || product.DownloadedPath != prepared.DownloadedPath {
		t.Fatalf("older receipt hid the prepared update: %+v", product)
	}
}

func TestSuccessfulInstallRecordMarksMatchingLatestVersionCurrent(t *testing.T) {
	checked := LauncherProductState{
		ID: "dev-helper", LatestVersion: "0.3.1-preview.3", UpdateStatus: "update_available",
		InstallStatus: "ready_to_install", DownloadedPath: `C:\cache\dev-helper-preview.3.exe`,
	}
	record := updatehistory.ProductRecord{
		ProductID: "dev-helper", InstalledVersion: "0.3.1-preview.3", Status: "installed",
		Message: "new version installed", ExecutablePath: `D:\helper\Dev Helper.exe`,
	}

	product := applyInstallRecord(checked, record)
	if product.UpdateStatus != "current" || product.InstallStatus != "installed" || product.DownloadedPath != "" {
		t.Fatalf("matching successful receipt did not finish the update: %+v", product)
	}
}

func TestExecutePreparedLauncherUpdateStartsOneShotBeforeQuitting(t *testing.T) {
	app := NewLauncherAppWithArgs(nil)
	release := updatecontract.ProductRelease{ProductID: "meow-launcher", Version: "0.6.7", InstallerType: "nsis-one-shot", ExecutableRelativePath: "Meow Launcher.exe"}
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &release}
	}
	app.authorizeInstall = func(updatecontract.ProductRelease) updateinstall.AuthorizationResult {
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationAuthorized, AuthorizationID: "grant"}
	}
	app.downloadRelease = func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		return updateinstall.DownloadResult{Path: filepath.Join(t.TempDir(), "launcher-one-shot.exe")}, nil
	}
	started := false
	app.executeInstaller = func(request updateinstall.ExecuteRequest) (updateinstall.ExecuteResult, error) {
		if request.Release.ProductID != "meow-launcher" || request.LauncherPID <= 0 {
			t.Fatalf("request=%+v", request)
		}
		started = true
		return updateinstall.ExecuteResult{Status: "self_update_installer_started", PID: 789}, nil
	}
	quit := make(chan struct{}, 1)
	app.quitLauncher = func() { quit <- struct{}{} }
	if _, err := app.PrepareProductUpdate("meow-launcher"); err != nil {
		t.Fatal(err)
	}
	product, err := app.ExecutePreparedUpdate("meow-launcher")
	if err != nil {
		t.Fatal(err)
	}
	if !started || product.InstallStatus != "self_update_installer_started" {
		t.Fatalf("started=%v product=%+v", started, product)
	}
	select {
	case <-quit:
	case <-time.After(time.Second):
		t.Fatal("Launcher did not quit after one-shot installer started")
	}
}

func TestStartupReconcilesReceiptAndLaunchesRecordedHelper(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	statePath := filepath.Join(machineRoot, "updates", "state.json")
	receiptPath := filepath.Join(machineRoot, "updates", "receipts", "tx-helper.json")
	helperPath := filepath.Join(machineRoot, "apps", "Dev Helper", "Dev Helper.exe")
	if err := os.MkdirAll(filepath.Dir(helperPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helperPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := updatehistory.RecordAttempt(statePath, updatehistory.ProductRecord{
		ProductID: "dev-helper", AttemptedVersion: "0.3.1", TransactionID: "tx-helper",
		ReceiptPath: receiptPath, ExecutablePath: helperPath,
	}); err != nil {
		t.Fatal(err)
	}
	receipt := updatehistory.Receipt{
		SchemaVersion: updatehistory.ReceiptSchemaVersion, TransactionID: "tx-helper", ProductID: "dev-helper", Version: "0.3.1",
		Status: "installed", Message: "installed", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: helperPath,
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	app.mu.Lock()
	app.updateChecks["brain2-manager"] = LauncherProductState{
		ID: "brain2-manager", UpdateStatus: "download_available", InstallStatus: "installer_started", InstallMessage: "stale pre-install state",
	}
	app.mu.Unlock()
	started := ""
	app.startManager = func(path string) error {
		started = path
		return nil
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	var helper LauncherProductState
	for _, product := range state.Products {
		if product.ID == "dev-helper" {
			helper = product
		}
	}
	if !helper.Installed || !helper.CanLaunch || helper.CurrentVersion != "0.3.1" || helper.InstallStatus != "installed" || helper.ExecutablePath != helperPath {
		t.Fatalf("helper=%+v", helper)
	}
	if err := app.LaunchProduct("dev-helper"); err != nil {
		t.Fatal(err)
	}
	if started != helperPath {
		t.Fatalf("started=%q", started)
	}
}

func TestStateRefreshReconcilesInstallerReceiptWithoutRestartingLauncher(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	statePath := filepath.Join(machineRoot, "updates", "state.json")
	receiptPath := filepath.Join(machineRoot, "updates", "receipts", "tx-manager.json")
	target := filepath.Join(machineRoot, "apps", "Brain2 Manager.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("installed manager"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := updatehistory.RecordAttempt(statePath, updatehistory.ProductRecord{
		ProductID: "brain2-manager", AttemptedVersion: "0.6.7", TransactionID: "tx-manager", ReceiptPath: receiptPath, ExecutablePath: target,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := updatehistory.RecordInstallerStarted(statePath, "brain2-manager", "tx-manager", 1234, "installer_started"); err != nil {
		t.Fatal(err)
	}
	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	receipt := updatehistory.Receipt{
		SchemaVersion: updatehistory.ReceiptSchemaVersion, TransactionID: "tx-manager", ProductID: "brain2-manager", Version: "0.6.7",
		Status: "installed", Message: "installed", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: target,
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range state.Products {
		if product.ID == "brain2-manager" {
			if product.InstallStatus != "installed" || product.CurrentVersion != "0.6.7" || product.ExecutablePath != target || product.UpdateStatus != "current" || product.DownloadedPath != "" {
				t.Fatalf("manager=%+v", product)
			}
			return
		}
	}
	t.Fatal("brain2-manager missing from Launcher state")
}

func TestFirstHelperInstallUsesSignedManifestTargetAndPersistsPendingTransaction(t *testing.T) {
	machineRoot := t.TempDir()
	localAppData := filepath.Join(machineRoot, "local")
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	t.Setenv("LOCALAPPDATA", localAppData)
	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	release := updatecontract.ProductRelease{
		ProductID: "dev-helper", Channel: "preview", Version: "0.3.1", InstallerType: "nsis",
		InstallScope: "per-user", InstallDirectoryName: "Dev Helper", ExecutableRelativePath: "Dev Helper.exe",
	}
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &release}
	}
	app.authorizeInstall = func(updatecontract.ProductRelease) updateinstall.AuthorizationResult {
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationAuthorized, AuthorizationID: "fresh-grant"}
	}
	app.downloadRelease = func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		return updateinstall.DownloadResult{Path: filepath.Join(machineRoot, "download.exe")}, nil
	}
	chosenDirectory := filepath.Join(machineRoot, "chosen")
	var dialogDefaultDirectory string
	app.chooseDirectory = func(_, defaultDirectory string) (string, error) {
		dialogDefaultDirectory = defaultDirectory
		return chosenDirectory, nil
	}
	var request updateinstall.ExecuteRequest
	app.executeInstaller = func(got updateinstall.ExecuteRequest) (updateinstall.ExecuteResult, error) {
		request = got
		return updateinstall.ExecuteResult{Status: "installer_started", PID: 4321}, nil
	}
	if _, err := app.ChooseProductInstallDirectory("dev-helper"); err != nil {
		t.Fatal(err)
	}
	if dialogDefaultDirectory != "" {
		t.Fatalf("directory picker should not impose a default path, got %q", dialogDefaultDirectory)
	}
	if _, err := app.PrepareProductUpdate("dev-helper"); err != nil {
		t.Fatal(err)
	}
	product, err := app.ExecutePreparedUpdate("dev-helper")
	if err != nil {
		t.Fatal(err)
	}
	systemRoot, layoutErr := installlayout.SystemRootFromSelection(chosenDirectory)
	if layoutErr != nil {
		t.Fatal(layoutErr)
	}
	wantDirectory, layoutErr := installlayout.DirectoryForSystemRoot(systemRoot, "dev-helper")
	if layoutErr != nil {
		t.Fatal(layoutErr)
	}
	wantTarget := filepath.Join(wantDirectory, "Dev Helper.exe")
	if product.InstallStatus != "installer_started" || request.TargetExecutable != wantTarget || request.TransactionID == "" || !filepath.IsAbs(request.ReceiptPath) {
		t.Fatalf("product=%+v request=%+v", product, request)
	}
	state, err := updatehistory.Load(filepath.Join(machineRoot, "updates", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	record := state.Products["dev-helper"]
	if record.Status != "installer_started" || record.AttemptedVersion != "0.3.1" || record.ExecutablePath != wantTarget || record.TransactionID != request.TransactionID || record.ReceiptPath != request.ReceiptPath {
		t.Fatalf("record=%+v", record)
	}
}

func TestChosenDirectorySurvivesUpdateCheckAndRefresh(t *testing.T) {
	machineRoot := t.TempDir()
	localAppData := filepath.Join(machineRoot, "local")
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	t.Setenv("LOCALAPPDATA", localAppData)
	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	release := updatecontract.ProductRelease{
		ProductID: "dev-helper", Channel: "preview", Version: "0.3.1", AccessPolicy: "public",
		InstallScope: "per-user", InstallDirectoryName: "Dev Helper", ExecutableRelativePath: "Dev Helper.exe",
	}
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &release}
	}
	custom := filepath.Join(machineRoot, "custom")
	app.chooseDirectory = func(_, _ string) (string, error) { return custom, nil }
	selected, err := app.ChooseProductInstallDirectory("dev-helper")
	if err != nil {
		t.Fatal(err)
	}
	systemRoot, layoutErr := installlayout.SystemRootFromSelection(custom)
	if layoutErr != nil {
		t.Fatal(layoutErr)
	}
	wantDirectory, layoutErr := installlayout.DirectoryForSystemRoot(systemRoot, "dev-helper")
	if layoutErr != nil {
		t.Fatal(layoutErr)
	}
	if selected.InstallDirectory != wantDirectory {
		t.Fatalf("custom directory was not selected: %+v", selected)
	}
	if _, err := app.CheckProductUpdate("dev-helper"); err != nil {
		t.Fatal(err)
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range state.Products {
		if product.ID == "dev-helper" && product.InstallDirectory != wantDirectory {
			t.Fatalf("refresh replaced custom directory: %+v", product)
		}
	}
}

func TestFirstInstallCannotStartBeforeUserChoosesDirectory(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	t.Setenv("LOCALAPPDATA", filepath.Join(machineRoot, "local"))
	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	release := updatecontract.ProductRelease{ProductID: "dev-helper", Channel: "preview", Version: "0.3.1", InstallerType: "nsis", AccessPolicy: "public", InstallScope: "per-user", InstallDirectoryName: "Dev Helper", ExecutableRelativePath: "Dev Helper.exe"}
	app.checkRelease = func(_, _, _ string) updatecontract.CheckResult {
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Release: &release}
	}
	app.authorizeInstall = func(updatecontract.ProductRelease) updateinstall.AuthorizationResult {
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationAuthorized}
	}
	app.downloadRelease = func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		return updateinstall.DownloadResult{Path: filepath.Join(machineRoot, "download.exe")}, nil
	}
	started := false
	app.executeInstaller = func(updateinstall.ExecuteRequest) (updateinstall.ExecuteResult, error) {
		started = true
		return updateinstall.ExecuteResult{Status: "installer_started", PID: 1}, nil
	}
	if _, err := app.PrepareProductUpdate("dev-helper"); err != nil {
		t.Fatal(err)
	}
	product, err := app.ExecutePreparedUpdate("dev-helper")
	if err != nil {
		t.Fatal(err)
	}
	if started || product.InstallStatus != "location_required" || product.ErrorCode != "INSTALL_LOCATION_REQUIRED" {
		t.Fatalf("started=%v product=%+v", started, product)
	}
}

func TestPublicReleaseIsAuthorizedWithoutLoginOrAuthorizationCode(t *testing.T) {
	result := defaultInstallAuthorization(updatecontract.ProductRelease{ProductID: "dev-helper", Version: "0.3.1", AccessPolicy: "public"})
	if result.Status != updateinstall.AuthorizationAuthorized || result.AuthorizationID != "" {
		t.Fatalf("result=%+v", result)
	}
	result = defaultInstallAuthorization(updatecontract.ProductRelease{ProductID: "dev-helper", Version: "0.3.1", AccessPolicy: "entitlement"})
	if result.Status != updateinstall.AuthorizationNotAuthenticated {
		t.Fatalf("entitlement result=%+v", result)
	}
}

func TestRequireManagerExecutableRejectsRelativePath(t *testing.T) {
	if _, err := requireManagerExecutable("Brain2 Manager.exe"); err == nil {
		t.Fatal("expected relative manager path to be rejected")
	}
}

func TestLaunchProductStartsResolvedBrain2Companion(t *testing.T) {
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	started := ""
	app := NewLauncherAppWithArgs(nil)
	app.startManager = func(path string) error {
		started = path
		return nil
	}
	if err := app.LaunchProduct("brain2-manager"); err != nil {
		t.Fatal(err)
	}
	if started != managerPath {
		t.Fatalf("expected Launcher to start %q, got %q", managerPath, started)
	}
}

func TestLaunchProductRejectsUnavailableCompanion(t *testing.T) {
	if err := NewLauncherApp().LaunchProduct("meow-protector"); err == nil {
		t.Fatal("expected a product without an installed executable to be rejected")
	}
}

func TestCheckProductUpdateUsesProductReleaseProvider(t *testing.T) {
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs(nil)
	app.checkRelease = func(productID, currentVersion, channel string) updatecontract.CheckResult {
		if productID != "brain2-manager" {
			t.Fatalf("unexpected product %q", productID)
		}
		return updatecontract.CheckResult{Status: updatecontract.StatusUpdateAvailable, Message: "发现新版本 0.6.7", Release: &updatecontract.ProductRelease{Version: "0.6.7", ReleaseNotes: "test"}}
	}
	product, err := app.CheckProductUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if product.UpdateStatus != "update_available" || product.LatestVersion != "0.6.7" || product.LastCheckedAt == "" {
		t.Fatalf("unexpected update result %#v", product)
	}
}

func TestLauncherReadsSharedLocalManifestWithoutUsingAuthorizationAsUpdateState(t *testing.T) {
	manifestPath, err := filepath.Abs(filepath.Join("testdata", "update", "manifest-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_UPDATE_MANIFEST", manifestPath)
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs([]string{"--update-product", "brain2-manager", "--return", "restart"})
	product, err := app.CheckProductUpdate("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if product.UpdateStatus != "update_available" || product.LatestVersion != "0.6.7-test" {
		t.Fatalf("unexpected update state %#v", product)
	}
	if product.AuthorizationStatus != "authorized" || product.AuthorizationMessage == "" {
		t.Fatalf("public release should be explicitly installable without login: %#v", product)
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.RequestedProductID != "brain2-manager" || state.RequestedReturn != "restart" {
		t.Fatalf("unexpected deep-link state %#v", state)
	}
}

func TestUninstallProductRemovesOnlyAllowlistedApplicationFile(t *testing.T) {
	managerPath := filepath.Join(t.TempDir(), brain2ManagerFilename)
	if err := os.WriteFile(managerPath, []byte("test executable placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_BRAIN2_MANAGER_PATH", managerPath)
	app := NewLauncherAppWithArgs(nil)
	plan, err := app.GetUninstallPlan("brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Installed || plan.ApplicationPath != managerPath || plan.CleanupAvailable {
		t.Fatalf("unexpected uninstall plan %#v", plan)
	}
	if _, err := app.UninstallProduct("brain2-manager", "wrong-confirmation"); err == nil {
		t.Fatal("expected mismatched confirmation to be rejected")
	}
	state, err := app.UninstallProduct("brain2-manager", "brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managerPath); !os.IsNotExist(err) {
		t.Fatalf("expected only application file to be removed, stat error %v", err)
	}
	for _, product := range state.Products {
		if product.ID == "brain2-manager" && product.Installed {
			t.Fatalf("expected Brain2 Manager to be uninstalled, got %#v", product)
		}
	}
}

func TestUninstallBundleInstalledHelperPreservesUnownedFilesAndClearsHistory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", filepath.Join(root, "machine"))
	installRoot := filepath.Join(root, "Dev Helper")
	helperPath := filepath.Join(installRoot, "Dev Helper.exe")
	appResource := filepath.Join(installRoot, "resources", "app.asar")
	userFile := filepath.Join(installRoot, "user-notes.txt")
	for file, content := range map[string]string{
		helperPath: "helper", appResource: "application", userFile: "must remain",
	} {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ownershipPath := filepath.Join(installRoot, ".meow-installed-files.json")
	ownership := map[string]any{
		"schema_version": "meow-installed-files/v1",
		"product_id":     "dev-helper",
		"version":        "0.3.1-preview.1",
		"files":          []string{"Dev Helper.exe", "resources/app.asar"},
	}
	ownershipJSON, err := json.Marshal(ownership)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownershipPath, ownershipJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	app := NewLauncherAppWithArgs(nil)
	receiptPath := filepath.Join(root, "helper-receipt.json")
	if _, err := updatehistory.RecordAttempt(app.historyPath, updatehistory.ProductRecord{
		ProductID: "dev-helper", AttemptedVersion: "0.3.1-preview.1", TransactionID: "tx-helper-uninstall",
		ReceiptPath: receiptPath, ExecutablePath: helperPath,
	}); err != nil {
		t.Fatal(err)
	}
	receiptJSON, err := json.Marshal(updatehistory.Receipt{
		SchemaVersion: updatehistory.ReceiptSchemaVersion, TransactionID: "tx-helper-uninstall", ProductID: "dev-helper",
		Version: "0.3.1-preview.1", Status: "installed", Message: "installed", CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: helperPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, receiptJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	app.historyReady = true
	app.reconcileUpdateHistory()
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	var helper LauncherProductState
	for _, product := range state.Products {
		if product.ID == "dev-helper" {
			helper = product
			break
		}
	}
	if !helper.Installed || !helper.CanUninstall || helper.ExecutablePath != helperPath {
		t.Fatalf("valid ownership manifest did not enable safe uninstall: %#v", helper)
	}

	plan, err := app.GetUninstallPlan("dev-helper")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Installed || !plan.CleanupAvailable || len(plan.RemoveItems) != 3 {
		t.Fatalf("unexpected uninstall plan %#v", plan)
	}
	if _, err := app.UninstallProduct("dev-helper", "dev-helper"); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{helperPath, appResource, ownershipPath} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("owned file still exists: %s (%v)", removed, err)
		}
	}
	if data, err := os.ReadFile(userFile); err != nil || string(data) != "must remain" {
		t.Fatalf("unowned user file changed: %q (%v)", data, err)
	}
	history, err := updatehistory.Load(app.historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := history.Products["dev-helper"]; exists {
		t.Fatalf("uninstalled Helper remains in update history: %#v", history.Products)
	}
}

func TestLauncherRequestOpensExactUpdateProduct(t *testing.T) {
	request := parseLauncherRequest([]string{"--update-product", "dev-helper", "--return", "restart"})
	if request.ProductID != "dev-helper" || request.ReturnAction != "restart" {
		t.Fatalf("unexpected request %#v", request)
	}
	request = parseLauncherRequest([]string{"meowlauncher://update?product=brain2-manager&return=restart"})
	if request.ProductID != "brain2-manager" || request.ReturnAction != "restart" {
		t.Fatalf("unexpected protocol request %#v", request)
	}
}

func TestResolveInstallTargetMigratesEveryProductIntoMeowSystem(t *testing.T) {
	tests := []struct {
		productID string
		existing  string
		relative  string
		want      string
	}{
		{productID: "meow-launcher", existing: `E:\launcher\Meow Launcher.exe`, relative: "Meow Launcher.exe", want: `E:\Meow System\Launcher\Meow Launcher.exe`},
		{productID: "brain2-manager", existing: `D:\manager\Brain2 Manager.exe`, relative: "Brain2 Manager.exe", want: `D:\Meow System\Brain2 Manager\Brain2 Manager.exe`},
		{productID: "dev-helper", existing: `D:\helper\Dev Helper.exe`, relative: "Dev Helper.exe", want: `D:\Meow System\Helper\Dev Helper.exe`},
	}
	for _, test := range tests {
		got, err := resolveInstallTarget(test.existing, "", updatecontract.ProductRelease{ProductID: test.productID, ExecutableRelativePath: test.relative})
		if err != nil {
			t.Fatalf("%s: %v", test.productID, err)
		}
		if got != test.want {
			t.Fatalf("%s target = %q, want %q", test.productID, got, test.want)
		}
	}
}

func TestResolveInstallTargetPreservesConfiguredCustomRootAndUsesItForLegacyMigration(t *testing.T) {
	release := updatecontract.ProductRelease{ProductID: "dev-helper", ExecutableRelativePath: "Dev Helper.exe"}
	customDirectory := `D:\Apps\Meow System\Helper`
	customExecutable := filepath.Join(customDirectory, "Dev Helper.exe")

	preserved, err := resolveInstallTarget(customExecutable, "", release)
	if err != nil {
		t.Fatal(err)
	}
	if preserved != customExecutable {
		t.Fatalf("custom target = %q, want %q", preserved, customExecutable)
	}

	migrated, err := resolveInstallTarget(`D:\helper\Dev Helper.exe`, customDirectory, release)
	if err != nil {
		t.Fatal(err)
	}
	if migrated != customExecutable {
		t.Fatalf("legacy migration target = %q, want %q", migrated, customExecutable)
	}
}

func TestStartupCleansOnlyOwnedLegacyFilesAfterCanonicalRestart(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	legacyRoot := filepath.Join(machineRoot, "legacy-helper")
	legacyExecutable := filepath.Join(legacyRoot, "Dev Helper.exe")
	legacyResource := filepath.Join(legacyRoot, "resources", "app.asar")
	userFile := filepath.Join(legacyRoot, "user-notes.txt")
	canonicalRoot := filepath.Join(machineRoot, "Meow System", "Helper")
	canonicalExecutable := filepath.Join(canonicalRoot, "Dev Helper.exe")
	writeOwnedProduct(t, legacyRoot, "dev-helper", legacyExecutable, []string{"Dev Helper.exe", "resources/app.asar"})
	writeOwnedProduct(t, canonicalRoot, "dev-helper", canonicalExecutable, []string{"Dev Helper.exe"})
	if err := os.MkdirAll(filepath.Dir(legacyResource), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyResource, []byte("old resource"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userFile, []byte("preserve me"), 0o600); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(machineRoot, "updates", "state.json")
	receiptPath := filepath.Join(machineRoot, "updates", "receipts", "tx-migrate-helper.json")
	if _, err := updatehistory.RecordAttempt(statePath, updatehistory.ProductRecord{
		ProductID: "dev-helper", AttemptedVersion: "0.3.1-preview.9", TransactionID: "tx-migrate-helper",
		ReceiptPath: receiptPath, ExecutablePath: canonicalExecutable, PreviousExecutablePath: legacyExecutable,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := updatehistory.RecordInstallerStarted(statePath, "dev-helper", "tx-migrate-helper", 9191, "installer_started"); err != nil {
		t.Fatal(err)
	}
	writeUpdateReceipt(t, receiptPath, updatehistory.Receipt{
		SchemaVersion: updatehistory.ReceiptSchemaVersion, TransactionID: "tx-migrate-helper", ProductID: "dev-helper",
		Version: "0.3.1-preview.9", Status: "installed", Message: "installed", Restarted: true,
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: canonicalExecutable,
	})

	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	for _, removed := range []string{legacyExecutable, legacyResource, filepath.Join(legacyRoot, ".meow-installed-files.json")} {
		if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned legacy file remains: %s (%v)", removed, err)
		}
	}
	if data, err := os.ReadFile(userFile); err != nil || string(data) != "preserve me" {
		t.Fatalf("unowned user file changed: %q (%v)", data, err)
	}
	if _, err := os.Stat(canonicalExecutable); err != nil {
		t.Fatalf("canonical executable changed: %v", err)
	}
	history, err := updatehistory.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if history.Products["dev-helper"].MigrationStatus != "completed" {
		t.Fatalf("migration not completed: %+v", history.Products["dev-helper"])
	}
}

func TestStartupReconcilesOldLauncherTargetThenCleansOwnedLegacyFiles(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	legacyRoot := filepath.Join(machineRoot, "legacy-launcher")
	legacyExecutable := filepath.Join(legacyRoot, "Meow Launcher.exe")
	legacyResource := filepath.Join(legacyRoot, "old-runtime.dll")
	userFile := filepath.Join(legacyRoot, "user-note.txt")
	canonicalRoot := filepath.Join(machineRoot, "Meow System", "Launcher")
	canonicalExecutable := filepath.Join(canonicalRoot, "Meow Launcher.exe")
	writeOwnedProduct(t, legacyRoot, "meow-launcher", legacyExecutable, []string{"Meow Launcher.exe", "old-runtime.dll"})
	if err := os.WriteFile(legacyResource, []byte("old owned runtime"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userFile, []byte("preserve me"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeOwnedProduct(t, canonicalRoot, "meow-launcher", canonicalExecutable, []string{"Meow Launcher.exe"})

	statePath := filepath.Join(machineRoot, "updates", "state.json")
	receiptPath := filepath.Join(machineRoot, "updates", "receipts", "tx-old-launcher-target.json")
	if _, err := updatehistory.RecordAttempt(statePath, updatehistory.ProductRecord{
		ProductID: "meow-launcher", AttemptedVersion: "0.6.7-preview.30", TransactionID: "tx-old-launcher-target",
		ReceiptPath: receiptPath, ExecutablePath: legacyExecutable,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := updatehistory.RecordInstallerStarted(statePath, "meow-launcher", "tx-old-launcher-target", 3030, "self_update_installer_started"); err != nil {
		t.Fatal(err)
	}
	writeUpdateReceipt(t, receiptPath, updatehistory.Receipt{
		SchemaVersion: updatehistory.ReceiptSchemaVersion, TransactionID: "tx-old-launcher-target", ProductID: "meow-launcher",
		Version: "0.6.7-preview.30", Status: "installed", Message: "installed", Restarted: true,
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: canonicalExecutable,
	})

	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	for _, removed := range []string{legacyExecutable, legacyResource, filepath.Join(legacyRoot, ".meow-installed-files.json")} {
		if _, err := os.Stat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned legacy file remains: %s (%v)", removed, err)
		}
	}
	if data, err := os.ReadFile(userFile); err != nil || string(data) != "preserve me" {
		t.Fatalf("unowned legacy file changed: %q (%v)", data, err)
	}
	history, err := updatehistory.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	record := history.Products["meow-launcher"]
	if record.ExecutablePath != canonicalExecutable || record.PreviousExecutablePath != legacyExecutable || record.MigrationStatus != "completed" {
		t.Fatalf("migration record=%+v", record)
	}
}

func TestMigrationKeepsLegacyVersionWhenCanonicalRestartFails(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	legacyRoot := filepath.Join(machineRoot, "legacy-launcher")
	legacyExecutable := filepath.Join(legacyRoot, "Meow Launcher.exe")
	canonicalRoot := filepath.Join(machineRoot, "Meow System", "Launcher")
	canonicalExecutable := filepath.Join(canonicalRoot, "Meow Launcher.exe")
	writeOwnedProduct(t, legacyRoot, "meow-launcher", legacyExecutable, []string{"Meow Launcher.exe"})
	writeOwnedProduct(t, canonicalRoot, "meow-launcher", canonicalExecutable, []string{"Meow Launcher.exe"})
	statePath := filepath.Join(machineRoot, "updates", "state.json")
	receiptPath := filepath.Join(machineRoot, "updates", "receipts", "tx-restart-failed.json")
	if _, err := updatehistory.RecordAttempt(statePath, updatehistory.ProductRecord{
		ProductID: "meow-launcher", AttemptedVersion: "0.6.7-preview.29", TransactionID: "tx-restart-failed",
		ReceiptPath: receiptPath, ExecutablePath: canonicalExecutable, PreviousExecutablePath: legacyExecutable,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := updatehistory.RecordInstallerStarted(statePath, "meow-launcher", "tx-restart-failed", 9292, "self_update_installer_started"); err != nil {
		t.Fatal(err)
	}
	writeUpdateReceipt(t, receiptPath, updatehistory.Receipt{
		SchemaVersion: updatehistory.ReceiptSchemaVersion, TransactionID: "tx-restart-failed", ProductID: "meow-launcher",
		Version: "0.6.7-preview.29", Status: "installed_restart_failed", Message: "restart failed", Restarted: false,
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: canonicalExecutable,
	})
	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	if data, err := os.ReadFile(legacyExecutable); err != nil || string(data) != "owned executable" {
		t.Fatalf("legacy fallback was changed: %q (%v)", data, err)
	}
	history, err := updatehistory.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if history.Products["meow-launcher"].MigrationStatus != "pending" {
		t.Fatalf("failed restart unexpectedly cleaned migration: %+v", history.Products["meow-launcher"])
	}
}

func TestMigrationRefusesLegacyCleanupWithoutValidOwnership(t *testing.T) {
	machineRoot := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machineRoot)
	legacyRoot := filepath.Join(machineRoot, "legacy-helper")
	legacyExecutable := filepath.Join(legacyRoot, "Dev Helper.exe")
	canonicalRoot := filepath.Join(machineRoot, "Meow System", "Helper")
	canonicalExecutable := filepath.Join(canonicalRoot, "Dev Helper.exe")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyExecutable, []byte("unowned legacy executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeOwnedProduct(t, canonicalRoot, "dev-helper", canonicalExecutable, []string{"Dev Helper.exe"})
	statePath := filepath.Join(machineRoot, "updates", "state.json")
	receiptPath := filepath.Join(machineRoot, "updates", "receipts", "tx-invalid-ownership.json")
	if _, err := updatehistory.RecordAttempt(statePath, updatehistory.ProductRecord{
		ProductID: "dev-helper", AttemptedVersion: "0.3.1-preview.9", TransactionID: "tx-invalid-ownership",
		ReceiptPath: receiptPath, ExecutablePath: canonicalExecutable, PreviousExecutablePath: legacyExecutable,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := updatehistory.RecordInstallerStarted(statePath, "dev-helper", "tx-invalid-ownership", 9393, "installer_started"); err != nil {
		t.Fatal(err)
	}
	writeUpdateReceipt(t, receiptPath, updatehistory.Receipt{
		SchemaVersion: updatehistory.ReceiptSchemaVersion, TransactionID: "tx-invalid-ownership", ProductID: "dev-helper",
		Version: "0.3.1-preview.9", Status: "installed", Message: "installed", Restarted: true,
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano), ExecutablePath: canonicalExecutable,
	})
	app := NewLauncherAppWithArgs(nil)
	app.startup(context.Background())
	if data, err := os.ReadFile(legacyExecutable); err != nil || string(data) != "unowned legacy executable" {
		t.Fatalf("unowned legacy executable was changed: %q (%v)", data, err)
	}
	history, err := updatehistory.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if history.Products["dev-helper"].MigrationStatus != "cleanup_failed" {
		t.Fatalf("invalid ownership did not block cleanup: %+v", history.Products["dev-helper"])
	}
}

func writeOwnedProduct(t *testing.T, root, productID, executable string, files []string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("owned executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]any{
		"schema_version": "meow-installed-files/v1",
		"product_id":     productID,
		"version":        "test",
		"files":          files,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".meow-installed-files.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeUpdateReceipt(t *testing.T, path string, receipt updatehistory.Receipt) {
	t.Helper()
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSecondLauncherInstanceHandsRequestToRunningLauncher(t *testing.T) {
	app := NewLauncherAppWithArgs([]string{"--update-product", "brain2-manager"})
	var received LauncherRequest
	app.secondInstanceHook = func(request LauncherRequest) { received = request }

	app.handleSecondInstanceLaunch([]string{"--update-product", "dev-helper", "--return", "restart"})
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if received.ProductID != "dev-helper" || received.ReturnAction != "restart" {
		t.Fatalf("second instance request was not forwarded: %#v", received)
	}
	if state.RequestedProductID != "dev-helper" || state.RequestedReturn != "restart" {
		t.Fatalf("running Launcher did not adopt the second request: %#v", state)
	}
}
