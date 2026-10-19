//go:build launcher

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"meow-launcher/internal/buildinfo"
	"meow-launcher/internal/desktopshortcut"
	"meow-launcher/internal/installerui"
	"meow-launcher/internal/installlayout"
	"meow-launcher/internal/launchlocation"
	"meow-launcher/internal/model"
	"meow-launcher/internal/processcontrol"
	"meow-launcher/internal/productinstaller"
	"meow-launcher/internal/settings"
	"meow-launcher/internal/updatecontract"
	"meow-launcher/internal/updatehistory"
	"meow-launcher/internal/updateinstall"
)

const brain2ManagerFilename = "Brain2 Manager v0.6.6 WIP.exe"

type LauncherProductState struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	ReleaseStatus        string `json:"release_status"`
	CatalogVersion       string `json:"catalog_version"`
	CurrentVersion       string `json:"current_version,omitempty"`
	Channel              string `json:"channel"`
	Installed            bool   `json:"installed"`
	ExecutablePath       string `json:"executable_path,omitempty"`
	CanLaunch            bool   `json:"can_launch"`
	UpdateStatus         string `json:"update_status"`
	UpdateMessage        string `json:"update_message"`
	LatestVersion        string `json:"latest_version,omitempty"`
	LastCheckedAt        string `json:"last_checked_at,omitempty"`
	CanUninstall         bool   `json:"can_uninstall"`
	ReleaseNotes         string `json:"release_notes,omitempty"`
	PublishedAt          string `json:"published_at,omitempty"`
	AuthorizationStatus  string `json:"authorization_status"`
	AuthorizationMessage string `json:"authorization_message"`
	InstallStatus        string `json:"install_status"`
	InstallMessage       string `json:"install_message"`
	DownloadedPath       string `json:"downloaded_path,omitempty"`
	InstallDirectory     string `json:"install_directory,omitempty"`
	ErrorCode            string `json:"error_code,omitempty"`
}

type LauncherDownloadProgress struct {
	ProductID  string `json:"product_id"`
	Phase      string `json:"phase"`
	Bytes      int64  `json:"bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

type ProductUninstallPlan struct {
	ProductID        string   `json:"product_id"`
	ProductName      string   `json:"product_name"`
	Installed        bool     `json:"installed"`
	CurrentVersion   string   `json:"current_version,omitempty"`
	ApplicationPath  string   `json:"application_path,omitempty"`
	RemoveItems      []string `json:"remove_items"`
	PreserveItems    []string `json:"preserve_items"`
	CleanupAvailable bool     `json:"cleanup_available"`
}

type LauncherSurfaceState struct {
	Products             []LauncherProductState `json:"products"`
	Authenticated        bool                   `json:"authenticated"`
	AccountStatus        string                 `json:"account_status"`
	MeowSystemRoot       string                 `json:"meow_system_root,omitempty"`
	MeowSystemStatus     string                 `json:"meow_system_status"`
	MeowSystemMessage    string                 `json:"meow_system_message"`
	LauncherBuild        string                 `json:"launcher_build"`
	DownloadService      string                 `json:"download_service"`
	RequestedProductID   string                 `json:"requested_product_id,omitempty"`
	RequestedReturn      string                 `json:"requested_return,omitempty"`
	UpdateHistoryStatus  string                 `json:"update_history_status"`
	UpdateHistoryMessage string                 `json:"update_history_message"`
}

type LauncherRequest struct {
	ProductID    string `json:"product_id"`
	ReturnAction string `json:"return_action"`
}

// LauncherApp is the lifecycle shell for the complete Meow product catalog.
// Product-specific workspaces, projects, Agent connections, health checks, and
// cross-product grants remain inside the corresponding product processes.
type LauncherApp struct {
	ctx                context.Context
	startManager       func(string) error
	checkRelease       func(string, string, string) updatecontract.CheckResult
	checkEntitlement   func(string) (string, string)
	authorizeInstall   func(updatecontract.ProductRelease) updateinstall.AuthorizationResult
	downloadRelease    func(updatecontract.ProductRelease) (updateinstall.DownloadResult, error)
	executeInstaller   func(updateinstall.ExecuteRequest) (updateinstall.ExecuteResult, error)
	closeProduct       func(context.Context, string) error
	chooseDirectory    func(string, string) (string, error)
	currentExecutable  func() (string, error)
	quitLauncher       func()
	removeProductFile  func(string) error
	mu                 sync.Mutex
	updateChecks       map[string]LauncherProductState
	releases           map[string]updatecontract.ProductRelease
	downloads          map[string]updateinstall.DownloadResult
	installTargets     map[string]string
	installRecords     map[string]updatehistory.ProductRecord
	historyPath        string
	historyReady       bool
	historyError       string
	request            LauncherRequest
	secondInstanceHook func(LauncherRequest)
}

func NewLauncherApp() *LauncherApp {
	return NewLauncherAppWithArgs(os.Args[1:])
}

func NewLauncherAppWithArgs(args []string) *LauncherApp {
	app := &LauncherApp{
		startManager:      startBrain2ManagerProcess,
		checkRelease:      defaultProductReleaseCheck,
		checkEntitlement:  defaultEntitlementCheck,
		authorizeInstall:  defaultInstallAuthorization,
		executeInstaller:  defaultInstallerExecution,
		closeProduct:      processcontrol.CloseExecutable,
		removeProductFile: os.Remove,
		currentExecutable: os.Executable,
		updateChecks:      map[string]LauncherProductState{},
		releases:          map[string]updatecontract.ProductRelease{},
		downloads:         map[string]updateinstall.DownloadResult{},
		installTargets:    map[string]string{},
		installRecords:    map[string]updatehistory.ProductRecord{},
		request:           parseLauncherRequest(args),
	}
	app.downloadRelease = func(release updatecontract.ProductRelease) (updateinstall.DownloadResult, error) {
		var progressMu sync.Mutex
		var lastProgressAt time.Time
		return defaultReleaseDownload(release, func(progress updateinstall.DownloadProgress) {
			progressMu.Lock()
			if progress.Phase == "downloading" && progress.Bytes > 0 && progress.Bytes < progress.TotalBytes && time.Since(lastProgressAt) < 100*time.Millisecond {
				progressMu.Unlock()
				return
			}
			lastProgressAt = time.Now()
			progressMu.Unlock()
			app.emitDownloadProgress(release.ProductID, progress)
		})
	}
	if machineRoot, err := settings.MachineRoot(); err == nil {
		app.historyPath = filepath.Join(machineRoot, "updates", "state.json")
	} else {
		app.historyError = err.Error()
	}
	app.quitLauncher = func() {
		if app.ctx != nil {
			wailsruntime.Quit(app.ctx)
		}
	}
	app.chooseDirectory = func(title, defaultDirectory string) (string, error) {
		selected, ok, err := installerui.ChooseFolderFrom(title, defaultDirectory)
		if err != nil || !ok {
			return "", err
		}
		return selected, nil
	}
	return app
}

func (a *LauncherApp) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	if executable, err := a.currentExecutable(); err == nil {
		_ = launchlocation.Save(executable)
	}
	a.historyReady = a.historyPath != ""
	a.reconcileUpdateHistoryWithCleanup(true)
}

func (a *LauncherApp) GetState() (LauncherSurfaceState, error) {
	// Reconcile on every state refresh so a completed child installer is
	// reflected without requiring the user to restart Launcher first.
	a.reconcileUpdateHistory()
	systemRoot, systemStatus, systemMessage := a.meowSystemConfiguration()
	products := make([]LauncherProductState, 0, len(model.Catalog()))
	managerPath, managerErr := locateBrain2Manager()
	for _, product := range model.Catalog() {
		item := LauncherProductState{
			ID:             product.ID,
			Name:           product.Name,
			Description:    product.Description,
			ReleaseStatus:  string(product.Status),
			CatalogVersion: product.Version,
			Channel:        product.Channel,
			UpdateStatus:   "not_checked",
			UpdateMessage:  "尚未检查产品应用更新",
			InstallStatus:  "not_started",
			InstallMessage: "尚未执行安装授权或下载",
		}
		item.AuthorizationStatus, item.AuthorizationMessage = a.checkEntitlement(product.ID)
		if product.Status == model.StatusPlanned {
			item.UpdateStatus = "planned"
			item.UpdateMessage = "产品仍在规划中"
		}
		if product.ID == "brain2-manager" && managerErr == nil {
			item.Installed = true
			item.CanLaunch = true
			item.ExecutablePath = managerPath
			item.InstallDirectory = filepath.Dir(managerPath)
			item.CurrentVersion = product.Version
			item.CanUninstall = true
		} else if product.ID == "meow-launcher" {
			item.Installed = true
			item.CurrentVersion = product.Version
			if executable, err := a.currentExecutable(); err == nil {
				item.ExecutablePath = executable
				item.InstallDirectory = filepath.Dir(executable)
			}
		} else if product.Status == model.StatusAvailable {
			item.UpdateStatus = "not_installed"
			item.UpdateMessage = "产品尚未安装"
		}
		a.mu.Lock()
		if selected, ok := a.installTargets[product.ID]; ok {
			item.InstallDirectory = selected
		}
		if checked, ok := a.updateChecks[product.ID]; ok {
			item = mergeCheckedProduct(item, checked)
		}
		if record, ok := a.installRecords[product.ID]; ok {
			// A verified installer receipt is authoritative. Apply it after the
			// pre-install check cache so stale "downloading/starting" state cannot
			// hide a completed installation.
			item = applyInstallRecord(item, record)
		}
		a.mu.Unlock()
		if systemStatus == "ready" {
			targetDirectory, targetErr := installlayout.DirectoryForSystemRoot(systemRoot, product.ID)
			if targetErr == nil {
				if !item.Installed {
					item.InstallDirectory = targetDirectory
					a.mu.Lock()
					a.installTargets[product.ID] = targetDirectory
					a.mu.Unlock()
				} else if item.ExecutablePath != "" && !installlayout.IsProductDirectory(filepath.Dir(item.ExecutablePath), product.ID) {
					// Keep a legacy install where it is until its next governed update.
					a.mu.Lock()
					a.installTargets[product.ID] = targetDirectory
					a.mu.Unlock()
				}
			}
		}
		products = append(products, item)
	}
	a.mu.Lock()
	request := a.request
	a.mu.Unlock()
	state := LauncherSurfaceState{
		Products:             products,
		Authenticated:        false,
		AccountStatus:        "公开 Preview · 当前无需登录",
		MeowSystemRoot:       systemRoot,
		MeowSystemStatus:     systemStatus,
		MeowSystemMessage:    systemMessage,
		LauncherBuild:        "v" + buildinfo.LauncherDisplayVersion,
		DownloadService:      "ready",
		RequestedProductID:   request.ProductID,
		RequestedReturn:      request.ReturnAction,
		UpdateHistoryStatus:  "ready",
		UpdateHistoryMessage: "安装记录与回执存储可用",
	}
	a.mu.Lock()
	if a.historyError != "" {
		state.UpdateHistoryStatus = "error"
		state.UpdateHistoryMessage = a.historyError
	}
	a.mu.Unlock()
	return state, nil
}

func (a *LauncherApp) meowSystemConfiguration() (string, string, string) {
	state, err := settings.Load()
	if err != nil {
		return "", "error", "无法读取 Meow System 安装位置：" + err.Error()
	}
	if root := strings.TrimSpace(state.MeowSystemRoot); root != "" {
		root = filepath.Clean(root)
		if _, err := installlayout.DirectoryForSystemRoot(root, "meow-launcher"); err != nil {
			return "", "error", "已保存的 Meow System 安装位置无效"
		}
		return root, "ready", "Meow 系列应用将统一安装到此目录"
	}
	executable, err := a.currentExecutable()
	if err == nil {
		if root, ok := installlayout.SystemRootFromProductDirectory(filepath.Dir(executable), "meow-launcher"); ok {
			if err := settings.SaveMeowSystemRoot(root); err != nil {
				return "", "error", "无法保存当前 Meow System 安装位置：" + err.Error()
			}
			return root, "ready", "已从 Launcher 当前安装位置自动识别"
		}
	}
	return "", "required", "首次安装其他应用前，请先选择 Meow System 的位置"
}

func (a *LauncherApp) reconcileUpdateHistory() {
	a.reconcileUpdateHistoryWithCleanup(false)
}

func (a *LauncherApp) reconcileUpdateHistoryWithCleanup(allowMigrationCleanup bool) {
	if !a.historyReady {
		return
	}
	state, err := updatehistory.ReconcilePending(a.historyPath)
	if err == nil && allowMigrationCleanup {
		state, err = a.reconcileInstallMigrations(state)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.historyError = err.Error()
		return
	}
	a.historyError = ""
	a.installRecords = state.Products
}

func (a *LauncherApp) reconcileInstallMigrations(state updatehistory.State) (updatehistory.State, error) {
	for productID, record := range state.Products {
		if (record.MigrationStatus != "pending" && record.MigrationStatus != "cleanup_failed") ||
			record.Status != "installed" || !record.ReceiptRestarted || record.PreviousExecutablePath == "" {
			continue
		}
		message := "旧目录中的已登记程序文件已清理；未登记文件和用户数据保持不变"
		status := "completed"
		if cleanupErr := a.cleanupLegacyInstallation(productID, record.PreviousExecutablePath, record.ExecutablePath); cleanupErr != nil {
			status = "cleanup_failed"
			message = "新版已在 Meow System 规范目录安装并启动；旧目录清理未完成：" + cleanupErr.Error()
		}
		updated, recordErr := updatehistory.RecordMigrationCleanup(a.historyPath, productID, record.TransactionID, status, message)
		if recordErr != nil {
			return state, recordErr
		}
		state = updated
	}
	return state, nil
}

func (a *LauncherApp) cleanupLegacyInstallation(productID, previousExecutable, currentExecutable string) error {
	previousExecutable = filepath.Clean(previousExecutable)
	currentExecutable = filepath.Clean(currentExecutable)
	if !filepath.IsAbs(previousExecutable) || !filepath.IsAbs(currentExecutable) || strings.EqualFold(previousExecutable, currentExecutable) {
		return errors.New("目录迁移记录无效")
	}
	if !installlayout.IsProductDirectory(filepath.Dir(currentExecutable), productID) {
		return errors.New("新版本不在 Meow System 规范目录，拒绝清理旧目录")
	}
	if _, err := productinstaller.InstalledFilesForExecutable(currentExecutable, productID); err != nil {
		return errors.New("新目录所有权清单未通过核验，拒绝清理旧目录")
	}
	owned, err := productinstaller.InstalledFilesForExecutable(previousExecutable, productID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Lstat(previousExecutable); errors.Is(statErr, os.ErrNotExist) {
				return nil
			}
		}
		return errors.New("旧目录所有权清单未通过核验，拒绝清理")
	}
	for _, removePath := range owned {
		if err := a.removeProductFile(removePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("删除已登记程序文件失败: %w", err)
		}
	}
	return nil
}

func applyInstallRecord(product LauncherProductState, record updatehistory.ProductRecord) LauncherProductState {
	if record.InstalledVersion != "" {
		product.CurrentVersion = record.InstalledVersion
	}
	if record.ExecutablePath != "" {
		if resolved, err := requireProductExecutable(record.ExecutablePath); err == nil {
			product.Installed = true
			product.CanLaunch = true
			product.ExecutablePath = resolved
			product.InstallDirectory = filepath.Dir(resolved)
			if product.ID != "meow-launcher" {
				if _, ownedErr := productinstaller.InstalledFilesForExecutable(resolved, product.ID); ownedErr == nil {
					product.CanUninstall = true
				}
			}
		}
	}
	hasNewerCheckedRelease := record.InstalledVersion != "" && product.LatestVersion != "" &&
		updatecontract.CompareVersions(record.InstalledVersion, product.LatestVersion) < 0
	recordConfirmsLatest := (record.Status == "installed" || record.Status == "installed_restart_failed") &&
		record.InstalledVersion != "" && product.LatestVersion != "" &&
		updatecontract.CompareVersions(record.InstalledVersion, product.LatestVersion) >= 0
	keepPreparedUpdate := (product.UpdateStatus == "update_available" || product.UpdateStatus == "download_available") &&
		(product.InstallStatus == "ready_to_install" || product.InstallStatus == "product_still_running") && !recordConfirmsLatest
	// A successful receipt proves which version is installed on disk; it does
	// not prove that this old version is still current. Keep a newer online
	// check (and any verified installer prepared for it) visible instead of
	// replacing it with the previous installation's "installed" state.
	if !keepPreparedUpdate {
		product.InstallStatus = record.Status
		product.InstallMessage = record.Message
	}
	if record.Status == "installed" || record.Status == "installed_restart_failed" {
		if !hasNewerCheckedRelease {
			product.UpdateStatus = "current"
			product.UpdateMessage = "已安装当前线上版本"
			product.DownloadedPath = ""
		}
		product.ErrorCode = ""
	}
	if record.ErrorCode != "" {
		product.ErrorCode = record.ErrorCode
	}
	if record.MigrationStatus == "pending" {
		product.InstallMessage = "新版已写入 Meow System 规范目录；旧目录将在 Launcher 下次启动确认后清理"
	} else if record.MigrationStatus == "cleanup_failed" {
		product.InstallMessage = record.MigrationMessage
		product.ErrorCode = "LEGACY_CLEANUP_PENDING"
	}
	return product
}

func (a *LauncherApp) CheckProductUpdate(productID string) (LauncherProductState, error) {
	product, err := a.productState(productID)
	if err != nil {
		return LauncherProductState{}, err
	}
	if product.ReleaseStatus == string(model.StatusPlanned) {
		return product, nil
	}
	product.LastCheckedAt = time.Now().Format("2006-01-02 15:04:05")
	check := a.checkRelease(product.ID, product.CurrentVersion, product.Channel)
	product.ErrorCode = check.ErrorCode
	product.UpdateMessage = check.Message
	if check.Release != nil {
		product.LatestVersion = check.Release.Version
		product.ReleaseNotes = check.Release.ReleaseNotes
		product.PublishedAt = check.Release.PublishedAt
		a.mu.Lock()
		a.releases[product.ID] = *check.Release
		a.mu.Unlock()
		if check.Release.AccessPolicy == "public" {
			product.AuthorizationStatus = updateinstall.AuthorizationAuthorized
			product.AuthorizationMessage = "公开版本，当前无需登录或授权码"
		}
	}
	switch check.Status {
	case updatecontract.StatusUpdateAvailable:
		if product.Installed {
			product.UpdateStatus = "update_available"
		} else {
			product.UpdateStatus = "download_available"
		}
	case updatecontract.StatusCurrent:
		if product.Installed {
			product.UpdateStatus = "current"
		} else {
			product.UpdateStatus = "download_available"
		}
	default:
		product.UpdateStatus = "check_unavailable"
	}
	if check.Release != nil && product.Installed && updatecontract.CompareVersions(product.CurrentVersion, check.Release.MinimumSupportedVersion) < 0 {
		product.ErrorCode = "MINIMUM_SUPPORTED_VERSION"
		product.UpdateMessage = "当前版本过旧；Launcher 将使用完整安装包更新，并保留旧版回滚文件"
	}
	if check.Release != nil && product.ID != "meow-launcher" && updatecontract.CompareVersions(buildinfo.LauncherVersion, check.Release.MinimumLauncherVersion) < 0 {
		product.ErrorCode = "MINIMUM_LAUNCHER_VERSION"
		product.UpdateStatus = "launcher_update_required"
		product.UpdateMessage = "需要先更新 Meow Launcher，才能安装这个产品版本"
	}
	a.mu.Lock()
	a.updateChecks[productID] = product
	a.mu.Unlock()
	return product, nil
}

func defaultProductReleaseCheck(productID, currentVersion, channel string) updatecontract.CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	source := strings.TrimSpace(os.Getenv("MEOW_UPDATE_MANIFEST"))
	if source == "" {
		source = buildinfo.UpdateManifestURL
	}
	return updatecontract.Check(ctx, source, productID, currentVersion, channel)
}

func defaultEntitlementCheck(productID string) (string, string) {
	return updateinstall.AuthorizationUnavailable, "尚未读取版本访问策略；检查产品更新后确认安装资格"
}

func defaultInstallAuthorization(release updatecontract.ProductRelease) updateinstall.AuthorizationResult {
	if release.AccessPolicy == "public" {
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationAuthorized, Message: "这是公开版本，当前无需登录或授权码"}
	}
	if release.AccessPolicy != "entitlement" {
		return updateinstall.AuthorizationResult{Status: updateinstall.AuthorizationUnavailable, ErrorCode: "AUTH_POLICY_INVALID", Message: "发布清单的安装访问策略无效"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := updateinstall.AuthorizationClient{
		Endpoint:     os.Getenv("MEOW_AUTHORIZATION_ENDPOINT"),
		SessionToken: os.Getenv("MEOW_LAUNCHER_SESSION_TOKEN"),
	}
	return client.Check(ctx, release.ProductID, release.Version)
}

func defaultReleaseDownload(release updatecontract.ProductRelease, progress func(updateinstall.DownloadProgress)) (updateinstall.DownloadResult, error) {
	dataRoot := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if dataRoot == "" || !filepath.IsAbs(dataRoot) {
		return updateinstall.DownloadResult{}, errors.New("Launcher 下载目录不可用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	return (updateinstall.Downloader{Progress: progress}).Download(ctx, release, filepath.Join(dataRoot, "MeowLauncher", "updates", "downloads"))
}

func (a *LauncherApp) emitDownloadProgress(productID string, progress updateinstall.DownloadProgress) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "launcher-download-progress", LauncherDownloadProgress{
		ProductID: productID, Phase: progress.Phase, Bytes: progress.Bytes, TotalBytes: progress.TotalBytes,
	})
}

func (a *LauncherApp) PrepareProductUpdate(productID string) (LauncherProductState, error) {
	product, err := a.CheckProductUpdate(productID)
	if err != nil {
		return LauncherProductState{}, err
	}
	if product.UpdateStatus != "update_available" && product.UpdateStatus != "download_available" {
		product.InstallStatus = "not_available"
		product.InstallMessage = "当前没有可准备安装的新版本"
		a.storeProductState(product)
		return product, nil
	}
	a.mu.Lock()
	release, ok := a.releases[product.ID]
	a.mu.Unlock()
	if !ok {
		return LauncherProductState{}, errors.New("更新清单没有可安装的产品记录")
	}
	authorization := a.authorizeInstall(release)
	product.AuthorizationStatus = authorization.Status
	product.AuthorizationMessage = authorization.Message
	product.ErrorCode = authorization.ErrorCode
	if authorization.Status != updateinstall.AuthorizationAuthorized {
		product.InstallStatus = "authorization_required"
		product.InstallMessage = authorization.Message
		a.storeProductState(product)
		return product, nil
	}
	product.InstallStatus = "downloading"
	product.InstallMessage = "授权已通过，正在下载并验证安装包"
	a.storeProductState(product)
	result, downloadErr := a.downloadRelease(release)
	if downloadErr != nil {
		product.InstallStatus = "download_failed"
		product.InstallMessage = downloadErr.Error()
		product.ErrorCode = updateinstall.CodeOf(downloadErr)
		a.storeProductState(product)
		return product, nil
	}
	product.InstallStatus = "ready_to_install"
	product.InstallMessage = "安装包已通过 HTTPS、SHA-256 和签名校验；尚未执行安装"
	product.DownloadedPath = result.Path
	product.ErrorCode = ""
	a.mu.Lock()
	a.downloads[product.ID] = result
	a.mu.Unlock()
	a.storeProductState(product)
	return product, nil
}

func defaultInstallerExecution(request updateinstall.ExecuteRequest) (updateinstall.ExecuteResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return (updateinstall.Executor{}).Execute(ctx, request)
}

func (a *LauncherApp) ExecutePreparedUpdate(productID string) (LauncherProductState, error) {
	product, err := a.productState(productID)
	if err != nil {
		return LauncherProductState{}, err
	}
	a.mu.Lock()
	release, hasRelease := a.releases[product.ID]
	download, hasDownload := a.downloads[product.ID]
	a.mu.Unlock()
	preparedStatus := product.InstallStatus == "ready_to_install" || product.InstallStatus == "product_still_running"
	if !preparedStatus || !hasRelease || !hasDownload {
		product.InstallStatus = "not_ready"
		product.InstallMessage = "请先完成当次授权、下载和校验"
		a.storeProductState(product)
		return product, nil
	}
	authorization := a.authorizeInstall(release)
	product.AuthorizationStatus = authorization.Status
	product.AuthorizationMessage = authorization.Message
	product.ErrorCode = authorization.ErrorCode
	if authorization.Status != updateinstall.AuthorizationAuthorized {
		product.InstallStatus = "authorization_required"
		product.InstallMessage = "安装前的再次授权核验未通过：" + authorization.Message
		a.storeProductState(product)
		return product, nil
	}
	a.mu.Lock()
	selectedInstallDirectory := a.installTargets[product.ID]
	a.mu.Unlock()
	if product.ExecutablePath == "" && selectedInstallDirectory == "" {
		product.InstallStatus = "location_required"
		product.InstallMessage = "首次安装前请选择安装目录"
		product.ErrorCode = "INSTALL_LOCATION_REQUIRED"
		a.storeProductState(product)
		return product, nil
	}
	targetExecutable, targetErr := resolveInstallTarget(product.ExecutablePath, selectedInstallDirectory, release)
	if targetErr != nil {
		product.InstallStatus = "install_failed"
		product.InstallMessage = targetErr.Error()
		product.ErrorCode = "INSTALL_TARGET_INVALID"
		a.storeProductState(product)
		return product, nil
	}
	transactionID, transactionErr := newUpdateTransactionID()
	if transactionErr != nil || a.historyPath == "" {
		product.InstallStatus = "install_failed"
		product.InstallMessage = "无法创建可追踪的安装事务"
		product.ErrorCode = "INSTALL_TRANSACTION_INVALID"
		a.storeProductState(product)
		return product, nil
	}
	receiptPath := filepath.Join(filepath.Dir(a.historyPath), "receipts", transactionID+".json")
	previousExecutable := ""
	if product.ExecutablePath != "" && !strings.EqualFold(filepath.Clean(product.ExecutablePath), filepath.Clean(targetExecutable)) {
		previousExecutable = filepath.Clean(product.ExecutablePath)
	}
	if a.historyReady {
		state, historyErr := updatehistory.RecordAttempt(a.historyPath, updatehistory.ProductRecord{
			ProductID: product.ID, AttemptedVersion: release.Version, TransactionID: transactionID,
			ReceiptPath: receiptPath, ExecutablePath: targetExecutable, PreviousExecutablePath: previousExecutable,
		})
		if historyErr != nil {
			product.InstallStatus = "install_failed"
			product.InstallMessage = "无法在启动安装程序前持久化更新事务：" + historyErr.Error()
			product.ErrorCode = "UPDATE_HISTORY_FAILED"
			a.storeProductState(product)
			return product, nil
		}
		a.mu.Lock()
		a.installRecords = state.Products
		a.mu.Unlock()
	}
	a.mu.Lock()
	returnAction := a.request.ReturnAction
	a.mu.Unlock()
	if returnAction == "" && product.Installed {
		returnAction = "restart"
	}
	if product.Installed && product.ID != "meow-launcher" {
		product.InstallStatus = "closing_product"
		product.InstallMessage = "正在请求当前产品正常退出；确认退出后再开始替换文件"
		product.ErrorCode = ""
		a.storeProductState(product)
		closeContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		closeErr := a.closeProduct(closeContext, product.ExecutablePath)
		cancel()
		if closeErr != nil {
			product.InstallStatus = "product_still_running"
			product.InstallMessage = "无法正常关闭当前产品。请保存工作、手动关闭所有该产品窗口后重试；Launcher 没有强制结束进程，也没有替换文件。"
			product.ErrorCode = "PRODUCT_STILL_RUNNING"
			if a.historyReady {
				if state, historyErr := updatehistory.RecordAttemptFailed(a.historyPath, product.ID, transactionID, product.ErrorCode, product.InstallMessage); historyErr == nil {
					a.mu.Lock()
					a.installRecords = state.Products
					a.mu.Unlock()
				}
			}
			a.storeProductState(product)
			return product, nil
		}
	}
	result, executeErr := a.executeInstaller(updateinstall.ExecuteRequest{
		Release:          release,
		InstallerPath:    download.Path,
		TransactionID:    transactionID,
		ReceiptPath:      receiptPath,
		ReturnAction:     returnAction,
		TargetExecutable: targetExecutable,
		LauncherPID:      os.Getpid(),
	})
	if executeErr != nil {
		product.InstallStatus = "install_failed"
		product.InstallMessage = executeErr.Error()
		product.ErrorCode = updateinstall.CodeOf(executeErr)
		if a.historyReady {
			if state, historyErr := updatehistory.RecordAttemptFailed(a.historyPath, product.ID, transactionID, product.ErrorCode, product.InstallMessage); historyErr == nil {
				a.mu.Lock()
				a.installRecords = state.Products
				a.mu.Unlock()
			}
		}
		a.storeProductState(product)
		return product, nil
	}
	product.InstallStatus = result.Status
	product.InstallMessage = "一次性安装程序已启动；最终结果由安装回执确认"
	product.ErrorCode = ""
	if a.historyReady {
		state, historyErr := updatehistory.RecordInstallerStarted(a.historyPath, product.ID, transactionID, result.PID, result.Status)
		if historyErr != nil {
			product.InstallMessage += "；Launcher 未能记录安装进程，但保留了待核验事务"
			product.ErrorCode = "UPDATE_HISTORY_FAILED"
		} else {
			a.mu.Lock()
			a.installRecords = state.Products
			a.mu.Unlock()
		}
	}
	a.storeProductState(product)
	if product.ID == "meow-launcher" {
		time.AfterFunc(250*time.Millisecond, a.quitLauncher)
	}
	return product, nil
}

func newUpdateTransactionID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "update-" + hex.EncodeToString(buffer), nil
}

func (a *LauncherApp) ChooseProductInstallDirectory(productID string) (LauncherProductState, error) {
	product, release, err := a.installDirectoryRelease(productID)
	if err != nil {
		return LauncherProductState{}, err
	}
	if product.Installed && product.ExecutablePath != "" {
		target, targetErr := resolveInstallTarget(product.ExecutablePath, "", release)
		if targetErr != nil {
			return LauncherProductState{}, targetErr
		}
		product.InstallDirectory = filepath.Dir(target)
		if strings.EqualFold(filepath.Clean(target), filepath.Clean(product.ExecutablePath)) {
			product.InstallMessage = "后续更新继续使用 Meow System 规范目录"
		} else {
			product.InstallMessage = "下次更新将安全迁移到 " + product.InstallDirectory
		}
		a.storeProductState(product)
		return product, nil
	}
	root, status, _ := a.meowSystemConfiguration()
	if status != "ready" {
		if _, err := a.ChooseMeowSystemRoot(); err != nil {
			return LauncherProductState{}, err
		}
		root, status, _ = a.meowSystemConfiguration()
	}
	if status != "ready" || root == "" {
		return product, nil
	}
	installDirectory, err := installlayout.DirectoryForSystemRoot(root, product.ID)
	if err != nil {
		return LauncherProductState{}, err
	}
	a.mu.Lock()
	a.installTargets[product.ID] = installDirectory
	a.mu.Unlock()
	product.InstallDirectory = installDirectory
	if product.InstallStatus != "ready_to_install" {
		product.InstallStatus = "location_selected"
		product.InstallMessage = "将安装到 " + installDirectory
	} else {
		product.InstallMessage = "安装包已验证；将安装到 " + installDirectory
	}
	product.ErrorCode = ""
	a.storeProductState(product)
	return product, nil
}

// ChooseMeowSystemRoot stores one device-local root for all Meow application
// installs. The picker selects its parent so users need not create the managed
// folder themselves.
func (a *LauncherApp) ChooseMeowSystemRoot() (LauncherSurfaceState, error) {
	currentRoot, _, _ := a.meowSystemConfiguration()
	defaultDirectory := currentRoot
	if strings.EqualFold(filepath.Base(defaultDirectory), installlayout.SystemDirectoryName) {
		defaultDirectory = filepath.Dir(defaultDirectory)
	}
	selected, err := a.chooseDirectory("选择 Meow System 的上级位置", defaultDirectory)
	if err != nil {
		return LauncherSurfaceState{}, err
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return a.GetState()
	}
	root, err := installlayout.SystemRootFromSelection(selected)
	if err != nil {
		return LauncherSurfaceState{}, err
	}
	if err := settings.SaveMeowSystemRoot(root); err != nil {
		return LauncherSurfaceState{}, err
	}
	a.mu.Lock()
	for _, product := range model.Catalog() {
		if directory, directoryErr := installlayout.DirectoryForSystemRoot(root, product.ID); directoryErr == nil {
			a.installTargets[product.ID] = directory
		}
	}
	a.mu.Unlock()
	return a.GetState()
}

func (a *LauncherApp) installDirectoryRelease(productID string) (LauncherProductState, updatecontract.ProductRelease, error) {
	product, err := a.productState(productID)
	if err != nil {
		return LauncherProductState{}, updatecontract.ProductRelease{}, err
	}
	a.mu.Lock()
	release, ok := a.releases[product.ID]
	a.mu.Unlock()
	if !ok {
		if _, err := a.CheckProductUpdate(product.ID); err != nil {
			return LauncherProductState{}, updatecontract.ProductRelease{}, err
		}
		a.mu.Lock()
		release, ok = a.releases[product.ID]
		a.mu.Unlock()
	}
	if !ok {
		return LauncherProductState{}, updatecontract.ProductRelease{}, errors.New("请先成功检查产品更新，再选择安装目录")
	}
	return product, release, nil
}

func (a *LauncherApp) storeSelectedInstallDirectory(product LauncherProductState, selected string) (LauncherProductState, error) {
	directory, err := installlayout.DirectoryFromSelection(selected, product.ID)
	if err != nil {
		return LauncherProductState{}, err
	}
	a.mu.Lock()
	a.installTargets[product.ID] = directory
	a.mu.Unlock()
	product.InstallDirectory = directory
	if product.InstallStatus != "ready_to_install" {
		product.InstallStatus = "location_selected"
		product.InstallMessage = "将安装到 " + directory
	} else {
		product.InstallMessage = "安装包已验证；将安装到 " + directory
	}
	product.ErrorCode = ""
	a.storeProductState(product)
	return product, nil
}

func resolveInstallTarget(existing, selectedDirectory string, release updatecontract.ProductRelease) (string, error) {
	if strings.TrimSpace(existing) != "" {
		if !filepath.IsAbs(existing) {
			return "", errors.New("现有产品应用路径不是绝对路径")
		}
		installRoot := filepath.Dir(filepath.Clean(existing))
		if !installlayout.IsProductDirectory(installRoot, release.ProductID) {
			if strings.TrimSpace(selectedDirectory) != "" && installlayout.IsProductDirectory(selectedDirectory, release.ProductID) {
				installRoot = filepath.Clean(selectedDirectory)
			} else {
				var err error
				installRoot, err = installlayout.DirectoryFromSelection(existing, release.ProductID)
				if err != nil {
					return "", err
				}
			}
		}
		target := filepath.Join(installRoot, filepath.FromSlash(release.ExecutableRelativePath))
		if !pathInsideInstallRoot(target, installRoot) {
			return "", errors.New("发布清单的应用路径超出 Meow System 产品目录")
		}
		return filepath.Clean(target), nil
	}
	if strings.TrimSpace(selectedDirectory) != "" {
		installRoot := filepath.Clean(selectedDirectory)
		if !installlayout.IsProductDirectory(installRoot, release.ProductID) {
			var err error
			installRoot, err = installlayout.DirectoryFromSelection(selectedDirectory, release.ProductID)
			if err != nil {
				return "", err
			}
		}
		target := filepath.Join(installRoot, filepath.FromSlash(release.ExecutableRelativePath))
		if !pathInsideInstallRoot(target, installRoot) {
			return "", errors.New("发布清单的应用路径超出 Meow System 产品目录")
		}
		return filepath.Clean(target), nil
	}
	var selection string
	switch release.InstallScope {
	case "per-user":
		local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if local == "" || !filepath.IsAbs(local) {
			return "", errors.New("发布清单要求按用户安装，但 LOCALAPPDATA 不可用")
		}
		selection = local
	case "per-machine":
		base := strings.TrimSpace(os.Getenv("ProgramW6432"))
		if base == "" {
			base = strings.TrimSpace(os.Getenv("ProgramFiles"))
		}
		if base == "" || !filepath.IsAbs(base) {
			return "", errors.New("发布清单要求按机器安装，但 Program Files 不可用")
		}
		selection = base
	default:
		return "", errors.New("发布清单没有受支持的 install_scope")
	}
	installRoot, err := installlayout.DirectoryFromSelection(selection, release.ProductID)
	if err != nil {
		return "", err
	}
	target := filepath.Join(installRoot, filepath.FromSlash(release.ExecutableRelativePath))
	if !pathInsideInstallRoot(target, installRoot) {
		return "", errors.New("发布清单的应用路径超出安装目录")
	}
	return filepath.Clean(target), nil
}

func pathInsideInstallRoot(target, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (a *LauncherApp) storeProductState(product LauncherProductState) {
	a.mu.Lock()
	a.updateChecks[product.ID] = product
	a.mu.Unlock()
}

func mergeCheckedProduct(base, checked LauncherProductState) LauncherProductState {
	base.UpdateStatus = checked.UpdateStatus
	base.UpdateMessage = checked.UpdateMessage
	base.LatestVersion = checked.LatestVersion
	base.LastCheckedAt = checked.LastCheckedAt
	base.ReleaseNotes = checked.ReleaseNotes
	base.PublishedAt = checked.PublishedAt
	base.AuthorizationStatus = checked.AuthorizationStatus
	base.AuthorizationMessage = checked.AuthorizationMessage
	base.InstallStatus = checked.InstallStatus
	base.InstallMessage = checked.InstallMessage
	base.DownloadedPath = checked.DownloadedPath
	base.InstallDirectory = checked.InstallDirectory
	base.ErrorCode = checked.ErrorCode
	return base
}

func (a *LauncherApp) GetUninstallPlan(productID string) (ProductUninstallPlan, error) {
	product, err := a.productState(productID)
	if err != nil {
		return ProductUninstallPlan{}, err
	}
	plan := ProductUninstallPlan{
		ProductID:      product.ID,
		ProductName:    product.Name,
		Installed:      product.Installed,
		CurrentVersion: product.CurrentVersion,
		PreserveItems: []string{
			"用户创建的内容与项目数据",
			"Brain2 记忆库及 .brain2 工作区数据",
			"Launcher 备份与产品账户记录",
		},
		CleanupAvailable: false,
	}
	if product.Installed {
		plan.ApplicationPath = product.ExecutablePath
		if owned, ownedErr := productinstaller.InstalledFilesForExecutable(product.ExecutablePath, product.ID); ownedErr == nil {
			plan.RemoveItems = owned
			plan.CleanupAvailable = true
		} else if product.ID == "brain2-manager" {
			// Compatibility for the existing pre-bundle Manager installation.
			plan.RemoveItems = []string{product.ExecutablePath}
		}
	}
	return plan, nil
}

func (a *LauncherApp) UninstallProduct(productID, confirmation string) (LauncherSurfaceState, error) {
	productID = canonicalProductID(productID)
	if confirmation != productID {
		return LauncherSurfaceState{}, errors.New("卸载确认不匹配")
	}
	plan, err := a.GetUninstallPlan(productID)
	if err != nil {
		return LauncherSurfaceState{}, err
	}
	if !plan.Installed || plan.ApplicationPath == "" {
		return LauncherSurfaceState{}, errors.New("这个产品当前未安装")
	}
	if productID == "meow-launcher" {
		return LauncherSurfaceState{}, errors.New("Launcher 不能在自身运行时卸载自己")
	}
	if len(plan.RemoveItems) == 0 {
		return LauncherSurfaceState{}, errors.New("当前预览尚未接入这个产品的安装清单")
	}
	for _, removePath := range plan.RemoveItems {
		if err := a.removeProductFile(removePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return LauncherSurfaceState{}, fmt.Errorf("卸载 %s 失败；请先关闭产品后重试：%w", plan.ProductName, err)
		}
	}
	// A Meow-created shortcut is removed only when it still points to this
	// exact installed entrypoint. A same-named user shortcut is preserved.
	_ = desktopshortcut.RemoveIfTarget(plan.ApplicationPath)
	a.mu.Lock()
	delete(a.updateChecks, productID)
	delete(a.installTargets, productID)
	if a.historyReady {
		if state, historyErr := updatehistory.RemoveProduct(a.historyPath, productID); historyErr == nil {
			a.installRecords = state.Products
		} else {
			a.historyError = historyErr.Error()
		}
	} else {
		delete(a.installRecords, productID)
	}
	a.mu.Unlock()
	return a.GetState()
}

func (a *LauncherApp) productState(productID string) (LauncherProductState, error) {
	productID = canonicalProductID(productID)
	state, err := a.GetState()
	if err != nil {
		return LauncherProductState{}, err
	}
	for _, product := range state.Products {
		if product.ID == productID {
			return product, nil
		}
	}
	return LauncherProductState{}, errors.New("未知产品")
}

func (a *LauncherApp) LaunchProduct(productID string) error {
	productID = canonicalProductID(productID)
	product, err := a.productState(productID)
	if err != nil {
		return err
	}
	if !product.Installed || !product.CanLaunch || product.ExecutablePath == "" {
		return errors.New("这个产品尚未安装，后续将由 Launcher 统一下载和安装")
	}
	return a.startManager(product.ExecutablePath)
}

func canonicalProductID(productID string) string {
	productID = strings.TrimSpace(productID)
	if strings.EqualFold(productID, "brain2") {
		return "brain2-manager"
	}
	return strings.ToLower(productID)
}

func parseLauncherRequest(args []string) LauncherRequest {
	request := LauncherRequest{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--update-product":
			if index+1 < len(args) {
				index++
				request.ProductID = canonicalProductID(args[index])
			}
		case "--return":
			if index+1 < len(args) {
				index++
				request.ReturnAction = strings.ToLower(strings.TrimSpace(args[index]))
			}
		default:
			if strings.HasPrefix(strings.ToLower(args[index]), "meowlauncher://") {
				if parsed, err := url.Parse(args[index]); err == nil && strings.EqualFold(parsed.Host, "update") {
					request.ProductID = canonicalProductID(parsed.Query().Get("product"))
					request.ReturnAction = strings.ToLower(strings.TrimSpace(parsed.Query().Get("return")))
				}
			}
		}
	}
	if request.ReturnAction != "restart" {
		request.ReturnAction = ""
	}
	return request
}

// handleSecondInstanceLaunch receives deep links and command-line update requests
// from a short-lived second Launcher process. The original process remains the
// sole owner of downloads and installation state, then comes to the foreground.
func (a *LauncherApp) handleSecondInstanceLaunch(args []string) {
	request := parseLauncherRequest(args)
	a.mu.Lock()
	a.request = request
	ctx := a.ctx
	hook := a.secondInstanceHook
	a.mu.Unlock()

	if hook != nil {
		hook(request)
		return
	}
	if ctx == nil {
		return
	}
	// A product-triggered update must be visibly handed over to Launcher. Showing
	// a hidden window alone is not enough on Windows: an already-running window
	// may remain minimised or behind the calling product, making an active
	// download look as if nothing happened.
	wailsruntime.WindowShow(ctx)
	wailsruntime.WindowUnminimise(ctx)
	bringLauncherWindowToFront()
	wailsruntime.EventsEmit(ctx, "launcher-update-request", request)
}

func startBrain2ManagerProcess(managerPath string) error {
	command := exec.Command(managerPath)
	command.Dir = filepath.Dir(managerPath)
	return command.Start()
}

func (a *LauncherApp) WindowMinimise() {
	if a.ctx != nil {
		wailsruntime.WindowMinimise(a.ctx)
	}
}

func (a *LauncherApp) WindowToggleMaximise() {
	if a.ctx != nil {
		wailsruntime.WindowToggleMaximise(a.ctx)
	}
}

func (a *LauncherApp) WindowClose() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

func locateBrain2Manager() (string, error) {
	if override := strings.TrimSpace(os.Getenv("MEOW_BRAIN2_MANAGER_PATH")); override != "" {
		return requireManagerExecutable(override)
	}
	executable, err := os.Executable()
	if err == nil {
		if candidate, candidateErr := requireManagerExecutable(filepath.Join(filepath.Dir(executable), brain2ManagerFilename)); candidateErr == nil {
			return candidate, nil
		}
		if candidate, candidateErr := requireManagerExecutable(filepath.Join(filepath.Dir(executable), "Brain2 Manager.exe")); candidateErr == nil {
			return candidate, nil
		}
	}
	workingDirectory, _ := os.Getwd()
	for _, candidate := range []string{
		filepath.Join(workingDirectory, "build", "bin", brain2ManagerFilename),
		filepath.Join(workingDirectory, "dist", brain2ManagerFilename),
	} {
		if resolved, candidateErr := requireManagerExecutable(candidate); candidateErr == nil {
			return resolved, nil
		}
	}
	return "", errors.New("Brain2 Manager 尚未安装或不在 Launcher 同目录")
}

func requireManagerExecutable(path string) (string, error) {
	return requireProductExecutable(path)
}

func requireProductExecutable(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return "", errors.New("产品应用路径必须是绝对路径")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("产品应用可执行文件不存在或不是普通文件")
	}
	return path, nil
}
