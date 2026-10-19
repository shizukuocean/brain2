package oneshot

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const PlanSchemaVersion = "meow-one-shot-install/v1"
const ReceiptSchemaVersion = "meow-install-receipt/v1"

var productIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)
var fileHashPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type FileEntry struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	SHA256     string `json:"sha256"`
}

type Plan struct {
	SchemaVersion     string      `json:"schema_version"`
	TransactionID     string      `json:"transaction_id"`
	ProductID         string      `json:"product_id"`
	Version           string      `json:"version"`
	TargetRoot        string      `json:"target_root"`
	BackupRoot        string      `json:"backup_root"`
	ReceiptPath       string      `json:"receipt_path"`
	WaitPIDs          []int       `json:"wait_pids,omitempty"`
	RestartExecutable string      `json:"restart_executable,omitempty"`
	RestartArguments  []string    `json:"restart_arguments,omitempty"`
	Files             []FileEntry `json:"files"`
	RemovePaths       []string    `json:"remove_paths,omitempty"`
}

type Receipt struct {
	SchemaVersion  string `json:"schema_version"`
	TransactionID  string `json:"transaction_id"`
	ProductID      string `json:"product_id"`
	Version        string `json:"version"`
	Status         string `json:"status"`
	ErrorCode      string `json:"error_code,omitempty"`
	Message        string `json:"message"`
	CompletedAt    string `json:"completed_at"`
	Restarted      bool   `json:"restarted"`
	ExecutablePath string `json:"executable_path,omitempty"`
}

func LoadPlan(path, expectedSHA256 string) (Plan, error) {
	if !filepath.IsAbs(path) {
		return Plan{}, installError("PLAN_INVALID", "安装计划必须使用绝对路径", nil)
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Plan{}, installError("PLAN_INVALID", "无法读取安装计划", err)
	}
	if expectedSHA256 != "" {
		if !fileHashPattern.MatchString(expectedSHA256) {
			return Plan{}, installError("PLAN_HASH_MISMATCH", "安装计划哈希格式无效", nil)
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(data))
		if !strings.EqualFold(actual, expectedSHA256) {
			return Plan{}, installError("PLAN_HASH_MISMATCH", "安装计划在启动后发生变化", nil)
		}
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, installError("PLAN_INVALID", "安装计划 JSON 无效", err)
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (p Plan) Validate() error {
	if p.SchemaVersion != PlanSchemaVersion {
		return installError("PLAN_INVALID", "安装计划 schema_version 不受支持", nil)
	}
	if strings.TrimSpace(p.TransactionID) == "" || !productIDPattern.MatchString(p.ProductID) || strings.TrimSpace(p.Version) == "" {
		return installError("PLAN_INVALID", "安装计划缺少事务、产品或版本身份", nil)
	}
	for name, value := range map[string]string{"target_root": p.TargetRoot, "backup_root": p.BackupRoot, "receipt_path": p.ReceiptPath} {
		if !filepath.IsAbs(value) {
			return installError("PLAN_INVALID", name+" 必须使用绝对路径", nil)
		}
	}
	targetRoot := filepath.Clean(p.TargetRoot)
	backupRoot := filepath.Clean(p.BackupRoot)
	if isBroadRoot(targetRoot) || isBroadRoot(backupRoot) {
		return installError("PLAN_INVALID", "安装目标或备份目录范围过宽", nil)
	}
	if samePath(targetRoot, backupRoot) || pathWithin(backupRoot, targetRoot) || pathWithin(targetRoot, backupRoot) {
		return installError("PLAN_INVALID", "安装目标与备份目录必须相互独立", nil)
	}
	if !strings.EqualFold(filepath.VolumeName(targetRoot), filepath.VolumeName(backupRoot)) {
		return installError("PLAN_INVALID", "备份目录必须与应用目录位于同一磁盘，才能原子恢复", nil)
	}
	if len(p.Files) == 0 {
		return installError("PLAN_INVALID", "安装计划没有应用文件", nil)
	}
	seen := map[string]bool{}
	for index, entry := range p.Files {
		if !filepath.IsAbs(entry.SourcePath) || !filepath.IsAbs(entry.TargetPath) {
			return installError("PLAN_INVALID", fmt.Sprintf("files[%d] 必须使用绝对路径", index), nil)
		}
		if !pathWithin(entry.TargetPath, targetRoot) {
			return installError("PLAN_INVALID", fmt.Sprintf("files[%d] 超出应用目录", index), nil)
		}
		if !fileHashPattern.MatchString(entry.SHA256) {
			return installError("PLAN_INVALID", fmt.Sprintf("files[%d].sha256 无效", index), nil)
		}
		key := strings.ToLower(filepath.Clean(entry.TargetPath))
		if seen[key] {
			return installError("PLAN_INVALID", "安装计划包含重复目标文件", nil)
		}
		seen[key] = true
	}
	for index, removePath := range p.RemovePaths {
		if !filepath.IsAbs(removePath) || !pathWithin(removePath, targetRoot) {
			return installError("PLAN_INVALID", fmt.Sprintf("remove_paths[%d] 超出应用目录", index), nil)
		}
		key := strings.ToLower(filepath.Clean(removePath))
		if seen[key] {
			return installError("PLAN_INVALID", "安装计划不能同时替换和删除同一文件", nil)
		}
		seen[key] = true
	}
	if p.RestartExecutable != "" && (!filepath.IsAbs(p.RestartExecutable) || !pathWithin(p.RestartExecutable, targetRoot)) {
		return installError("PLAN_INVALID", "重启程序必须位于应用目录内", nil)
	}
	return nil
}

func VerifyFile(path, expected string) error {
	info, err := os.Lstat(filepath.Clean(path))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return installError("PAYLOAD_INVALID", "安装载荷不是普通文件", err)
	}
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return installError("PAYLOAD_INVALID", "无法读取安装载荷", err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return installError("PAYLOAD_INVALID", "无法计算安装载荷哈希", err)
	}
	if actual := fmt.Sprintf("%x", digest.Sum(nil)); !strings.EqualFold(actual, expected) {
		return installError("HASH_MISMATCH", "HASH_MISMATCH：安装载荷与发布计划不一致", nil)
	}
	return nil
}

func WriteReceipt(path string, receipt Receipt) error {
	receipt.SchemaVersion = ReceiptSchemaVersion
	receipt.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data, 0o600)
}

type codedInstallError struct {
	code, message string
	cause         error
}

func (e codedInstallError) Error() string { return e.message }
func (e codedInstallError) Unwrap() error { return e.cause }
func installError(code, message string, cause error) error {
	return codedInstallError{code: code, message: message, cause: cause}
}

func CodeOf(err error) string {
	var target codedInstallError
	if errors.As(err, &target) {
		return target.code
	}
	return "INSTALL_FAILED"
}

func pathWithin(candidate, root string) bool {
	candidate = filepath.Clean(candidate)
	root = filepath.Clean(root)
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func isBroadRoot(path string) bool {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume != "" {
		return samePath(path, volume+string(filepath.Separator))
	}
	return path == string(filepath.Separator) || path == "."
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return installError("RECEIPT_FAILED", "回执路径必须是绝对路径", nil)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".meow-receipt-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	backupPath := temporaryPath + ".previous"
	hadExisting := false
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return installError("RECEIPT_FAILED", "已有回执不是普通文件", nil)
		}
		if err := os.Rename(path, backupPath); err != nil {
			return err
		}
		hadExisting = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if hadExisting {
			_ = os.Rename(backupPath, path)
		}
		return err
	}
	if hadExisting {
		_ = os.Remove(backupPath)
	}
	return nil
}
