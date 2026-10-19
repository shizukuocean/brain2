package productinstaller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"meow-launcher/internal/desktopshortcut"
	"meow-launcher/internal/installerbundle"
	"meow-launcher/internal/installlayout"
	"meow-launcher/internal/oneshot"
)

type Invocation struct {
	ProductID        string
	Version          string
	TransactionID    string
	ReceiptPath      string
	WaitPID          int
	ReturnAction     string
	TargetExecutable string
	RestartLauncher  bool
	InitialInstall   bool
}

type Options struct {
	Apply               oneshot.ApplyOptions
	ReceiptRoot         string
	SkipDesktopShortcut bool
	ShortcutWriter      func(desktopshortcut.Request) (string, error)
}

type InitialLauncherMetadata struct {
	Version                string
	ExecutableRelativePath string
}

const installedFilesSchemaVersion = "meow-installed-files/v1"

type installedFilesManifest struct {
	SchemaVersion string   `json:"schema_version"`
	ProductID     string   `json:"product_id"`
	Version       string   `json:"version"`
	Files         []string `json:"files"`
}

type installFile struct {
	SourcePath   string
	RelativePath string
	SHA256       string
}

func Install(ctx context.Context, installerPath string, invocation Invocation, options Options) (receipt oneshot.Receipt, err error) {
	defer func() {
		if err == nil || !filepath.IsAbs(invocation.ReceiptPath) || strings.TrimSpace(invocation.TransactionID) == "" || strings.TrimSpace(invocation.ProductID) == "" || strings.TrimSpace(invocation.Version) == "" {
			return
		}
		failure := oneshot.Receipt{
			TransactionID: invocation.TransactionID, ProductID: invocation.ProductID, Version: invocation.Version,
			Status: "failed", ErrorCode: oneshot.CodeOf(err), Message: err.Error(), ExecutablePath: invocation.TargetExecutable,
		}
		_ = oneshot.WriteReceipt(invocation.ReceiptPath, failure)
		receipt = failure
	}()
	bundle, err := installerbundle.Open(installerPath)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	defer bundle.Close()
	if err := validateInvocation(bundle.Manifest, invocation); err != nil {
		return oneshot.Receipt{}, err
	}
	invocation, err = resolveLegacyLauncherMigration(invocation, bundle.Manifest)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	targetRoot, err := targetRoot(invocation.TargetExecutable, bundle.Manifest.ExecutableRelativePath)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	extractRoot, err := os.MkdirTemp("", ".meow-installer-"+bundle.Manifest.ProductID+"-*")
	if err != nil {
		return oneshot.Receipt{}, err
	}
	defer os.RemoveAll(extractRoot)
	extracted, err := bundle.Extract(extractRoot)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	installFiles, err := mapInstallFiles(targetRoot, invocation.TargetExecutable, bundle.Manifest, extracted)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	removePaths, err := staleInstalledFiles(targetRoot, bundle.Manifest.ProductID, installFiles)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	installedManifestSource, installedManifestHash, err := writeInstalledFilesManifest(extractRoot, bundle.Manifest, installFiles)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	plan := oneshot.Plan{
		SchemaVersion: oneshot.PlanSchemaVersion, TransactionID: invocation.TransactionID,
		ProductID: bundle.Manifest.ProductID, Version: bundle.Manifest.Version,
		TargetRoot: targetRoot, BackupRoot: filepath.Join(filepath.Dir(targetRoot), ".meow-update-backups", bundle.Manifest.ProductID, invocation.TransactionID),
		ReceiptPath: invocation.ReceiptPath,
		RemovePaths: removePaths,
	}
	if invocation.WaitPID > 0 {
		plan.WaitPIDs = []int{invocation.WaitPID}
	}
	if invocation.ReturnAction == "restart" {
		plan.RestartExecutable = filepath.Clean(invocation.TargetExecutable)
	}
	for _, file := range installFiles {
		plan.Files = append(plan.Files, oneshot.FileEntry{
			SourcePath: file.SourcePath,
			TargetPath: filepath.Join(targetRoot, filepath.FromSlash(file.RelativePath)),
			SHA256:     file.SHA256,
		})
	}
	plan.Files = append(plan.Files, oneshot.FileEntry{
		SourcePath: installedManifestSource,
		TargetPath: filepath.Join(targetRoot, installerbundle.InstalledFilesManifestName),
		SHA256:     installedManifestHash,
	})
	applyOptions := options.Apply
	previousAfterCommit := applyOptions.AfterCommit
	applyOptions.AfterCommit = func() error {
		if previousAfterCommit != nil {
			if err := previousAfterCommit(); err != nil {
				return err
			}
		}
		if options.SkipDesktopShortcut {
			return nil
		}
		writer := options.ShortcutWriter
		if writer == nil {
			writer = desktopshortcut.Ensure
		}
		_, err := writer(desktopshortcut.Request{
			TargetExecutable: filepath.Clean(invocation.TargetExecutable),
			Description:      "打开 " + strings.TrimSuffix(filepath.Base(invocation.TargetExecutable), filepath.Ext(invocation.TargetExecutable)),
		})
		return err
	}
	return oneshot.Apply(ctx, plan, applyOptions)
}

// resolveLegacyLauncherMigration is the compatibility bridge for Launchers
// released before the governed Meow System layout existed. The old Launcher
// still supplies its in-place executable as the target, so the new complete
// one-shot installer must move only a verifiably owned legacy installation to
// the canonical directory. The legacy files are intentionally left untouched
// here; the restarted new Launcher removes them only after reconciling the
// successful receipt and validating both ownership manifests.
func resolveLegacyLauncherMigration(invocation Invocation, manifest installerbundle.Manifest) (Invocation, error) {
	if manifest.ProductID != "meow-launcher" || !manifest.SelfUpdate || invocation.InitialInstall ||
		invocation.WaitPID <= 0 || !invocation.RestartLauncher {
		return invocation, nil
	}
	legacyExecutable := filepath.Clean(invocation.TargetExecutable)
	if installlayout.IsProductDirectory(filepath.Dir(legacyExecutable), "meow-launcher") {
		return invocation, nil
	}
	if _, err := InstalledFilesForExecutable(legacyExecutable, "meow-launcher"); err != nil {
		// Pre-bundle Launchers have no trustworthy ownership boundary. Preserve
		// their established in-place behavior instead of guessing what to move.
		return invocation, nil
	}
	canonicalRoot, err := canonicalLauncherMigrationRoot(legacyExecutable)
	if err != nil {
		return invocation, err
	}
	canonicalExecutable := filepath.Join(canonicalRoot, filepath.FromSlash(manifest.ExecutableRelativePath))
	if err := requireEmptyMigrationDestination(canonicalRoot); err != nil {
		return invocation, err
	}
	invocation.TargetExecutable = filepath.Clean(canonicalExecutable)
	return invocation, nil
}

func canonicalLauncherMigrationRoot(legacyExecutable string) (string, error) {
	// Release E2E processes opt into an isolated machine root and desktop.
	// Production Launchers never set the desktop test root and therefore always
	// resolve from the real volume containing the legacy executable.
	if strings.TrimSpace(os.Getenv("MEOW_DESKTOP_SHORTCUT_TEST_ROOT")) != "" {
		machineRoot := filepath.Clean(strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_MACHINE_ROOT")))
		if !filepath.IsAbs(machineRoot) || !pathInsideRoot(legacyExecutable, machineRoot) {
			return "", errors.New("隔离 Launcher 迁移目标必须位于绝对的 MEOW_LAUNCHER_MACHINE_ROOT 内")
		}
		return filepath.Join(machineRoot, installlayout.SystemDirectoryName, "Launcher"), nil
	}
	return installlayout.DirectoryFromSelection(legacyExecutable, "meow-launcher")
}

func requireEmptyMigrationDestination(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法核验 Launcher 规范目录: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Launcher 规范目录必须是普通目录，不能是文件或符号链接")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("无法读取 Launcher 规范目录: %w", err)
	}
	if len(entries) != 0 {
		return errors.New("Launcher 规范目录已有内容；为避免覆盖另一个实例，已取消自动迁移")
	}
	return nil
}

func pathInsideRoot(candidate, root string) bool {
	candidate = filepath.Clean(candidate)
	root = filepath.Clean(root)
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// InspectInitialLauncherBundle verifies that a user-opened package is the
// non-resident Launcher bundle, before any install location is requested.
func InspectInitialLauncherBundle(installerPath string) (InitialLauncherMetadata, error) {
	bundle, err := installerbundle.Open(installerPath)
	if err != nil {
		return InitialLauncherMetadata{}, err
	}
	defer bundle.Close()
	if bundle.Manifest.ProductID != "meow-launcher" || !bundle.Manifest.SelfUpdate {
		return InitialLauncherMetadata{}, errors.New("只有 Meow Launcher 完整安装包可以直接执行首次安装")
	}
	return InitialLauncherMetadata{
		Version: bundle.Manifest.Version, ExecutableRelativePath: bundle.Manifest.ExecutableRelativePath,
	}, nil
}

// InstallInitialLauncher performs a first installation from the same complete
// package later used for one-shot self-update. It never upgrades an existing
// target: subsequent replacement must come from the running Launcher with an
// exact wait PID.
func InstallInitialLauncher(ctx context.Context, installerPath, selectedDirectory string, options Options) (oneshot.Receipt, error) {
	metadata, err := InspectInitialLauncherBundle(installerPath)
	if err != nil {
		return oneshot.Receipt{}, err
	}
	selectedDirectory = filepath.Clean(strings.TrimSpace(selectedDirectory))
	if !filepath.IsAbs(selectedDirectory) || filepath.Dir(selectedDirectory) == selectedDirectory {
		return oneshot.Receipt{}, errors.New("请选择一个具体的绝对安装目录，不能直接安装到磁盘根目录")
	}
	if !installlayout.IsProductDirectory(selectedDirectory, "meow-launcher") {
		return oneshot.Receipt{}, errors.New("Launcher 必须安装在 Meow System\\Launcher 目录")
	}
	info, err := os.Lstat(selectedDirectory)
	if err != nil {
		return oneshot.Receipt{}, fmt.Errorf("无法读取所选安装目录: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return oneshot.Receipt{}, errors.New("所选安装位置必须是普通目录，不能是文件或符号链接")
	}
	targetExecutable := filepath.Join(selectedDirectory, filepath.FromSlash(metadata.ExecutableRelativePath))
	if _, err := os.Lstat(targetExecutable); err == nil {
		return oneshot.Receipt{}, errors.New("所选目录中已经存在 Meow Launcher；后续更新必须从已安装的 Launcher 发起")
	} else if !errors.Is(err, os.ErrNotExist) {
		return oneshot.Receipt{}, fmt.Errorf("无法核验目标 Launcher: %w", err)
	}
	if _, err := os.Lstat(filepath.Join(selectedDirectory, installerbundle.InstalledFilesManifestName)); err == nil {
		return oneshot.Receipt{}, errors.New("所选目录已有 Meow 安装记录；请启动现有 Launcher 更新或选择其他目录")
	} else if !errors.Is(err, os.ErrNotExist) {
		return oneshot.Receipt{}, fmt.Errorf("无法核验所选目录的安装记录: %w", err)
	}

	receiptRoot := strings.TrimSpace(options.ReceiptRoot)
	if receiptRoot == "" {
		machineRoot := strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_MACHINE_ROOT"))
		if machineRoot != "" {
			if !filepath.IsAbs(machineRoot) {
				return oneshot.Receipt{}, errors.New("MEOW_LAUNCHER_MACHINE_ROOT 必须是绝对路径")
			}
			receiptRoot = filepath.Join(machineRoot, "updates", "receipts")
		} else {
			cacheRoot, cacheErr := os.UserCacheDir()
			if cacheErr != nil {
				return oneshot.Receipt{}, fmt.Errorf("无法定位本机安装记录目录: %w", cacheErr)
			}
			receiptRoot = filepath.Join(cacheRoot, "MeowLauncher", "updates", "receipts")
		}
	}
	if !filepath.IsAbs(receiptRoot) {
		return oneshot.Receipt{}, errors.New("首次安装回执目录必须是绝对路径")
	}
	transactionID, err := randomTransactionID()
	if err != nil {
		return oneshot.Receipt{}, err
	}
	receiptPath := filepath.Join(receiptRoot, transactionID+".json")
	return Install(ctx, installerPath, Invocation{
		ProductID: "meow-launcher", Version: metadata.Version, TransactionID: transactionID,
		ReceiptPath: receiptPath, ReturnAction: "restart", TargetExecutable: targetExecutable,
		RestartLauncher: true, InitialInstall: true,
	}, options)
}

func randomTransactionID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("无法创建首次安装事务身份: %w", err)
	}
	return fmt.Sprintf("setup-%x", value), nil
}

func mapInstallFiles(targetRoot, targetExecutable string, manifest installerbundle.Manifest, extracted []installerbundle.ExtractedFile) ([]installFile, error) {
	mapped := make([]installFile, 0, len(extracted))
	seen := map[string]bool{}
	for _, file := range extracted {
		relative := file.RelativePath
		if strings.EqualFold(relative, manifest.ExecutableRelativePath) {
			mappedRelative, err := filepath.Rel(targetRoot, filepath.Clean(targetExecutable))
			if err != nil || mappedRelative == "." || mappedRelative == ".." || strings.HasPrefix(mappedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(mappedRelative) {
				return nil, errors.New("现有产品入口不在安装目录内")
			}
			relative = filepath.ToSlash(mappedRelative)
		}
		key := strings.ToLower(relative)
		if seen[key] {
			return nil, errors.New("安装载荷映射到重复的目标文件")
		}
		seen[key] = true
		mapped = append(mapped, installFile{SourcePath: file.SourcePath, RelativePath: relative, SHA256: file.SHA256})
	}
	return mapped, nil
}

func staleInstalledFiles(targetRoot, productID string, extracted []installFile) ([]string, error) {
	manifestPath := filepath.Join(targetRoot, installerbundle.InstalledFilesManifestName)
	info, err := os.Lstat(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		return nil, errors.New("已有应用文件清单无效；为保护目录内容，安装已停止")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var previous installedFilesManifest
	if err := json.Unmarshal(data, &previous); err != nil || previous.SchemaVersion != installedFilesSchemaVersion || previous.ProductID != productID {
		return nil, errors.New("已有应用文件清单身份无效；为保护目录内容，安装已停止")
	}
	current := map[string]bool{}
	for _, file := range extracted {
		current[strings.ToLower(file.RelativePath)] = true
	}
	seen := map[string]bool{}
	remove := []string{}
	for _, relative := range previous.Files {
		relative = path.Clean(strings.ReplaceAll(relative, "\\", "/"))
		if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || path.IsAbs(relative) || strings.Contains(relative, ":") {
			return nil, errors.New("已有应用文件清单包含越界路径；为保护目录内容，安装已停止")
		}
		key := strings.ToLower(relative)
		if seen[key] {
			return nil, errors.New("已有应用文件清单包含重复路径")
		}
		seen[key] = true
		if !current[key] {
			target := filepath.Join(targetRoot, filepath.FromSlash(relative))
			rel, relErr := filepath.Rel(targetRoot, target)
			if relErr != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return nil, errors.New("已有应用文件清单路径超出安装目录")
			}
			remove = append(remove, target)
		}
	}
	sort.Strings(remove)
	return remove, nil
}

// InstalledFilesForExecutable returns only application files previously
// written by a Meow bundle. It walks upward from the executable to support a
// nested entrypoint without guessing or recursively claiming the chosen root.
func InstalledFilesForExecutable(executablePath, productID string) ([]string, error) {
	executablePath = filepath.Clean(executablePath)
	if !filepath.IsAbs(executablePath) {
		return nil, errors.New("应用入口必须是绝对路径")
	}
	root := filepath.Dir(executablePath)
	for depth := 0; depth < 8; depth++ {
		manifestPath := filepath.Join(root, installerbundle.InstalledFilesManifestName)
		info, err := os.Lstat(manifestPath)
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
				return nil, errors.New("应用文件清单无效")
			}
			data, readErr := os.ReadFile(manifestPath)
			if readErr != nil {
				return nil, readErr
			}
			var record installedFilesManifest
			if jsonErr := json.Unmarshal(data, &record); jsonErr != nil || record.SchemaVersion != installedFilesSchemaVersion || record.ProductID != productID {
				return nil, errors.New("应用文件清单身份无效")
			}
			seen := map[string]bool{}
			owned := make([]string, 0, len(record.Files)+1)
			entrypointOwned := false
			for _, relative := range record.Files {
				relative = path.Clean(strings.ReplaceAll(relative, "\\", "/"))
				if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || path.IsAbs(relative) || strings.Contains(relative, ":") {
					return nil, errors.New("应用文件清单包含越界路径")
				}
				key := strings.ToLower(relative)
				if seen[key] {
					return nil, errors.New("应用文件清单包含重复路径")
				}
				seen[key] = true
				absolute := filepath.Join(root, filepath.FromSlash(relative))
				rel, relErr := filepath.Rel(root, absolute)
				if relErr != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
					return nil, errors.New("应用文件清单路径超出安装目录")
				}
				if strings.EqualFold(absolute, executablePath) {
					entrypointOwned = true
				}
				owned = append(owned, absolute)
			}
			if !entrypointOwned {
				return nil, errors.New("应用文件清单未登记当前产品入口")
			}
			owned = append(owned, manifestPath)
			return owned, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	return nil, os.ErrNotExist
}

func writeInstalledFilesManifest(extractRoot string, manifest installerbundle.Manifest, extracted []installFile) (string, string, error) {
	record := installedFilesManifest{SchemaVersion: installedFilesSchemaVersion, ProductID: manifest.ProductID, Version: manifest.Version}
	for _, file := range extracted {
		record.Files = append(record.Files, file.RelativePath)
	}
	sort.Strings(record.Files)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", "", err
	}
	source := filepath.Join(extractRoot, installerbundle.InstalledFilesManifestName)
	if err := os.WriteFile(source, append(data, '\n'), 0o600); err != nil {
		return "", "", err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(append(data, '\n')))
	return source, digest, nil
}

func validateInvocation(manifest installerbundle.Manifest, invocation Invocation) error {
	if strings.TrimSpace(invocation.ProductID) == "" || strings.TrimSpace(invocation.Version) == "" || strings.TrimSpace(invocation.TransactionID) == "" {
		return errors.New("安装调用缺少产品、版本或事务身份")
	}
	if manifest.ProductID != invocation.ProductID || manifest.Version != invocation.Version {
		return errors.New("安装调用与安装包内置的产品或版本不一致")
	}
	if !filepath.IsAbs(invocation.ReceiptPath) || !filepath.IsAbs(invocation.TargetExecutable) {
		return errors.New("安装回执和目标应用必须使用绝对路径")
	}
	if invocation.ReturnAction != "" && invocation.ReturnAction != "restart" {
		return errors.New("安装完成后的返回动作无效")
	}
	if manifest.SelfUpdate {
		if manifest.ProductID != "meow-launcher" || !invocation.RestartLauncher || invocation.ReturnAction != "restart" {
			return errors.New("Launcher 自更新必须等待当前 Launcher 退出并在安装后重启")
		}
		if invocation.InitialInstall {
			if invocation.WaitPID != 0 {
				return errors.New("Launcher 首次安装不能等待或复用其他 Launcher 进程")
			}
			if _, err := os.Lstat(invocation.TargetExecutable); err == nil {
				return errors.New("Launcher 首次安装不能替换已有程序；请从现有 Launcher 发起更新")
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("无法核验 Launcher 首次安装目标: %w", err)
			}
		} else if invocation.WaitPID <= 0 {
			return errors.New("Launcher 自更新必须等待当前 Launcher 的准确进程")
		}
	} else if invocation.WaitPID != 0 || invocation.RestartLauncher || invocation.InitialInstall {
		return errors.New("普通产品安装包不能声明 Launcher 自更新参数")
	}
	return nil
}

func targetRoot(targetExecutable, executableRelativePath string) (string, error) {
	target := filepath.Clean(targetExecutable)
	if !filepath.IsAbs(target) {
		return "", errors.New("目标应用必须是绝对路径")
	}
	root := filepath.Dir(target)
	entryDirectory := path.Dir(strings.ReplaceAll(executableRelativePath, "\\", "/"))
	if entryDirectory != "." {
		for range strings.Split(entryDirectory, "/") {
			root = filepath.Dir(root)
		}
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("目标应用路径与安装包入口不一致")
	}
	if volume := filepath.VolumeName(root); volume != "" && strings.EqualFold(filepath.Clean(root), volume+string(filepath.Separator)) {
		return "", errors.New("安装目录不能是磁盘根目录")
	}
	if root == filepath.Dir(root) {
		return "", errors.New("安装目录范围过宽")
	}
	return root, nil
}
