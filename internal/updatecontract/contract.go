package updatecontract

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = "meow-update-manifest/v1"

const (
	StatusNotChecked          = "not_checked"
	StatusSourceNotConfigured = "source_not_configured"
	StatusCheckFailed         = "check_failed"
	StatusManifestInvalid     = "manifest_invalid"
	StatusProductNotFound     = "product_not_found"
	StatusCurrent             = "current"
	StatusUpdateAvailable     = "update_available"
)

type Signature struct {
	Method    string `json:"method"`
	Required  bool   `json:"required"`
	Publisher string `json:"publisher,omitempty"`
}

type Rollback struct {
	Strategy                 string `json:"strategy"`
	PreviousVersionSupported bool   `json:"previous_version_supported"`
	BackupRequired           bool   `json:"backup_required"`
}

type ProductRelease struct {
	ProductID               string    `json:"product_id"`
	Channel                 string    `json:"channel"`
	Version                 string    `json:"version"`
	PublishedAt             string    `json:"published_at"`
	ReleaseNotes            string    `json:"release_notes"`
	DownloadURL             string    `json:"download_url"`
	InstallerType           string    `json:"installer_type"`
	AccessPolicy            string    `json:"access_policy"`
	InstallScope            string    `json:"install_scope"`
	InstallDirectoryName    string    `json:"install_directory_name"`
	ExecutableRelativePath  string    `json:"executable_relative_path"`
	SHA256                  string    `json:"sha256"`
	Signature               Signature `json:"signature"`
	MinimumLauncherVersion  string    `json:"minimum_launcher_version"`
	MinimumSupportedVersion string    `json:"minimum_supported_version"`
	Mandatory               bool      `json:"mandatory"`
	DataMigrationVersion    string    `json:"data_migration_version"`
	Rollback                Rollback  `json:"rollback"`
}

type Manifest struct {
	SchemaVersion string           `json:"schema_version"`
	ManifestURL   string           `json:"manifest_url"`
	GeneratedAt   string           `json:"generated_at"`
	Products      []ProductRelease `json:"products"`
}

type CheckResult struct {
	Status         string          `json:"status"`
	ErrorCode      string          `json:"error_code,omitempty"`
	Message        string          `json:"message"`
	CurrentVersion string          `json:"current_version"`
	CheckedAt      string          `json:"checked_at"`
	Release        *ProductRelease `json:"release,omitempty"`
}

var (
	sha256Pattern  = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)
)

func Check(ctx context.Context, source, productID, currentVersion, channel string) CheckResult {
	result := CheckResult{
		Status:         StatusNotChecked,
		CurrentVersion: currentVersion,
		CheckedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	source = strings.TrimSpace(source)
	if source == "" {
		result.Status = StatusSourceNotConfigured
		result.ErrorCode = "UPDATE_SOURCE_NOT_CONFIGURED"
		result.Message = "在线产品版本源尚未配置"
		return result
	}
	manifest, err := Load(ctx, source)
	if err != nil {
		result.ErrorCode = CodeOf(err)
		result.Status = StatusCheckFailed
		if result.ErrorCode == "UPDATE_MANIFEST_INVALID" {
			result.Status = StatusManifestInvalid
		}
		result.Message = err.Error()
		return result
	}
	release, ok := manifest.Find(productID, channel)
	if !ok {
		result.Status = StatusProductNotFound
		result.ErrorCode = "UPDATE_PRODUCT_NOT_FOUND"
		result.Message = "发布清单中没有这个产品或通道"
		return result
	}
	result.Release = &release
	if CompareVersions(release.Version, currentVersion) > 0 {
		result.Status = StatusUpdateAvailable
		result.Message = fmt.Sprintf("发现新版本 %s", release.Version)
	} else {
		result.Status = StatusCurrent
		result.Message = "当前已是最新产品版本"
	}
	return result
}

func Load(ctx context.Context, source string) (Manifest, error) {
	data, err := readSource(ctx, strings.TrimSpace(source))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, codedError("UPDATE_MANIFEST_INVALID", "更新清单 JSON 无效", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return codedError("UPDATE_MANIFEST_INVALID", "更新清单 schema_version 不受支持", nil)
	}
	if err := validateHTTPSURL(m.ManifestURL, "manifest_url"); err != nil {
		return codedError("UPDATE_MANIFEST_INVALID", err.Error(), err)
	}
	if _, err := time.Parse(time.RFC3339, m.GeneratedAt); err != nil {
		return codedError("UPDATE_MANIFEST_INVALID", "更新清单 generated_at 必须是 RFC3339 时间", err)
	}
	if len(m.Products) == 0 {
		return codedError("UPDATE_MANIFEST_INVALID", "更新清单没有产品", nil)
	}
	seen := map[string]bool{}
	for index, product := range m.Products {
		if err := product.Validate(); err != nil {
			return codedError("UPDATE_MANIFEST_INVALID", fmt.Sprintf("products[%d]: %s", index, err.Error()), err)
		}
		key := strings.ToLower(product.ProductID + "\x00" + product.Channel)
		if seen[key] {
			return codedError("UPDATE_MANIFEST_INVALID", "更新清单包含重复的 product_id + channel", nil)
		}
		seen[key] = true
	}
	return nil
}

func (p ProductRelease) Validate() error {
	for name, value := range map[string]string{
		"product_id": p.ProductID, "channel": p.Channel, "version": p.Version,
		"release_notes": p.ReleaseNotes, "download_url": p.DownloadURL,
		"installer_type": p.InstallerType, "minimum_launcher_version": p.MinimumLauncherVersion,
		"access_policy": p.AccessPolicy,
		"install_scope": p.InstallScope, "install_directory_name": p.InstallDirectoryName,
		"executable_relative_path":  p.ExecutableRelativePath,
		"minimum_supported_version": p.MinimumSupportedVersion, "data_migration_version": p.DataMigrationVersion,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s 不能为空", name)
		}
	}
	if _, err := time.Parse(time.RFC3339, p.PublishedAt); err != nil {
		return errors.New("published_at 必须是 RFC3339 时间")
	}
	if !sha256Pattern.MatchString(p.SHA256) {
		return errors.New("sha256 必须是 64 位十六进制")
	}
	for name, value := range map[string]string{
		"version": p.Version, "minimum_launcher_version": p.MinimumLauncherVersion, "minimum_supported_version": p.MinimumSupportedVersion,
	} {
		if !versionPattern.MatchString(value) {
			return fmt.Errorf("%s 版本格式无效", name)
		}
	}
	if p.Signature.Method == "" || (p.Signature.Required && strings.TrimSpace(p.Signature.Publisher) == "") {
		return errors.New("signature 必须声明 method；强制签名时还需 publisher")
	}
	if p.Rollback.Strategy == "" {
		return errors.New("rollback.strategy 不能为空")
	}
	if p.InstallScope != "per-user" && p.InstallScope != "per-machine" {
		return errors.New("install_scope 必须是 per-user 或 per-machine")
	}
	if p.AccessPolicy != "public" && p.AccessPolicy != "entitlement" {
		return errors.New("access_policy 必须是 public 或 entitlement")
	}
	installerType := strings.ToLower(strings.TrimSpace(p.InstallerType))
	standardInstaller := installerType == "meow-bundle" || installerType == "nsis"
	oneShotInstaller := installerType == "meow-bundle-one-shot" || installerType == "nsis-one-shot"
	if !standardInstaller && !oneShotInstaller {
		return errors.New("installer_type 不受当前 Launcher 支持")
	}
	if p.ProductID == "meow-launcher" && !oneShotInstaller {
		return errors.New("Meow Launcher 必须使用非驻留的一次性自更新安装包")
	}
	if p.ProductID != "meow-launcher" && !standardInstaller {
		return errors.New("普通产品不能使用 Launcher 自更新安装包")
	}
	if strings.EqualFold(p.Channel, "stable") && !p.Signature.Required {
		return errors.New("stable 通道必须要求可信代码签名")
	}
	if strings.TrimSpace(p.InstallDirectoryName) == "" || p.InstallDirectoryName == "." || strings.ContainsAny(p.InstallDirectoryName, `/\\`) {
		return errors.New("install_directory_name 必须是单层目录名")
	}
	executableRelative := filepath.Clean(strings.TrimSpace(p.ExecutableRelativePath))
	if executableRelative == "." || filepath.IsAbs(executableRelative) || executableRelative == ".." || strings.HasPrefix(executableRelative, ".."+string(filepath.Separator)) || !strings.EqualFold(filepath.Ext(executableRelative), ".exe") {
		return errors.New("executable_relative_path 必须是安装目录内的相对 EXE 路径")
	}
	return validateHTTPSURL(p.DownloadURL, "download_url")
}

func validateHTTPSURL(value, field string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" {
		return fmt.Errorf("%s 必须使用 HTTPS", field)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s 不能内嵌账号或令牌", field)
	}
	return nil
}

func (m Manifest) Find(productID, channel string) (ProductRelease, bool) {
	productID = strings.TrimSpace(productID)
	channel = strings.TrimSpace(channel)
	for _, product := range m.Products {
		if strings.EqualFold(product.ProductID, productID) && strings.EqualFold(product.Channel, channel) {
			return product, true
		}
	}
	return ProductRelease{}, false
}

func CompareVersions(left, right string) int {
	leftCore, leftPre := versionParts(left)
	rightCore, rightPre := versionParts(right)
	max := len(leftCore)
	if len(rightCore) > max {
		max = len(rightCore)
	}
	for index := 0; index < max; index++ {
		var l, r int
		if index < len(leftCore) {
			l = leftCore[index]
		}
		if index < len(rightCore) {
			r = rightCore[index]
		}
		if l < r {
			return -1
		}
		if l > r {
			return 1
		}
	}
	if leftPre == rightPre {
		return 0
	}
	if leftPre == "" {
		return 1
	}
	if rightPre == "" {
		return -1
	}
	return comparePrerelease(leftPre, rightPre)
}

func comparePrerelease(left, right string) int {
	leftIdentifiers := strings.Split(left, ".")
	rightIdentifiers := strings.Split(right, ".")
	max := len(leftIdentifiers)
	if len(rightIdentifiers) > max {
		max = len(rightIdentifiers)
	}
	for index := 0; index < max; index++ {
		if index >= len(leftIdentifiers) {
			return -1
		}
		if index >= len(rightIdentifiers) {
			return 1
		}
		if result := compareNaturalIdentifier(leftIdentifiers[index], rightIdentifiers[index]); result != 0 {
			return result
		}
	}
	return 0
}

func compareNaturalIdentifier(left, right string) int {
	if left == right {
		return 0
	}
	leftNumeric := allDigits(left)
	rightNumeric := allDigits(right)
	if leftNumeric && rightNumeric {
		return compareNumericText(left, right)
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}
	leftParts := naturalParts(left)
	rightParts := naturalParts(right)
	max := len(leftParts)
	if len(rightParts) > max {
		max = len(rightParts)
	}
	for index := 0; index < max; index++ {
		if index >= len(leftParts) {
			return -1
		}
		if index >= len(rightParts) {
			return 1
		}
		if leftParts[index] == rightParts[index] {
			continue
		}
		if allDigits(leftParts[index]) && allDigits(rightParts[index]) {
			return compareNumericText(leftParts[index], rightParts[index])
		}
		if leftParts[index] < rightParts[index] {
			return -1
		}
		return 1
	}
	return 0
}

func naturalParts(value string) []string {
	if value == "" {
		return nil
	}
	parts := make([]string, 0, 4)
	start := 0
	digit := value[0] >= '0' && value[0] <= '9'
	for index := 1; index < len(value); index++ {
		currentDigit := value[index] >= '0' && value[index] <= '9'
		if currentDigit != digit {
			parts = append(parts, value[start:index])
			start = index
			digit = currentDigit
		}
	}
	return append(parts, value[start:])
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func compareNumericText(left, right string) int {
	left = strings.TrimLeft(left, "0")
	right = strings.TrimLeft(right, "0")
	if left == "" {
		left = "0"
	}
	if right == "" {
		right = "0"
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func VerifyFileSHA256(path, expected string) error {
	if !sha256Pattern.MatchString(expected) {
		return codedError("HASH_MISMATCH", "HASH_MISMATCH：期望哈希格式无效", nil)
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return codedError("HASH_MISMATCH", "HASH_MISMATCH：无法读取下载文件", err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return codedError("HASH_MISMATCH", "HASH_MISMATCH：无法计算下载文件哈希", err)
	}
	actual := fmt.Sprintf("%x", digest.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return codedError("HASH_MISMATCH", "HASH_MISMATCH：下载文件与发布清单不一致", nil)
	}
	return nil
}

func versionParts(value string) ([]int, string) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(value), "v"))
	value = strings.ReplaceAll(value, " ", "-")
	parts := strings.SplitN(value, "-", 2)
	core := strings.Split(parts[0], ".")
	numbers := make([]int, len(core))
	for index, part := range core {
		numbers[index], _ = strconv.Atoi(part)
	}
	pre := ""
	if len(parts) == 2 {
		pre = parts[1]
	}
	return numbers, pre
}

func readSource(ctx context.Context, source string) ([]byte, error) {
	if parsed, err := url.Parse(source); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		if parsed.Scheme != "https" {
			return nil, codedError("UPDATE_SOURCE_INSECURE", "线上更新清单必须使用 HTTPS", nil)
		}
		if preferPlatformManifestRead(parsed) {
			data, err := platformManifestRead(ctx, source, 2<<20)
			if err != nil {
				return nil, codedError("UPDATE_CHECK_FAILED", "无法读取在线更新清单："+err.Error(), err)
			}
			return data, nil
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, codedError("UPDATE_CHECK_FAILED", "无法创建更新请求", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, codedError("UPDATE_CHECK_FAILED", "无法读取在线更新清单", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, codedError("UPDATE_CHECK_FAILED", fmt.Sprintf("更新源返回 HTTP %d", response.StatusCode), nil)
		}
		return io.ReadAll(io.LimitReader(response.Body, 2<<20))
	}
	if strings.HasPrefix(strings.ToLower(source), "file://") {
		parsed, err := url.Parse(source)
		if err != nil {
			return nil, codedError("UPDATE_CHECK_FAILED", "本地更新清单路径无效", err)
		}
		source = filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/"))
		if parsed.Host != "" {
			source = filepath.Join(parsed.Host, source)
		}
	}
	if !filepath.IsAbs(source) {
		return nil, codedError("UPDATE_CHECK_FAILED", "本地更新清单必须使用绝对路径", nil)
	}
	data, err := os.ReadFile(filepath.Clean(source))
	if err != nil {
		return nil, codedError("UPDATE_CHECK_FAILED", "无法读取本地更新清单", err)
	}
	return data, nil
}

type contractError struct {
	code, message string
	cause         error
}

func (e contractError) Error() string { return e.message }
func (e contractError) Unwrap() error { return e.cause }
func codedError(code, message string, cause error) error {
	return contractError{code: code, message: message, cause: cause}
}
func CodeOf(err error) string {
	var target contractError
	if errors.As(err, &target) {
		return target.code
	}
	return "UPDATE_CHECK_FAILED"
}
