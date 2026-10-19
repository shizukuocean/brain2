package updatehistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"meow-launcher/internal/installlayout"
)

const StateSchemaVersion = "meow-update-state/v1"
const ReceiptSchemaVersion = "meow-install-receipt/v1"

var productIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

type ProductRecord struct {
	ProductID              string `json:"product_id"`
	InstalledVersion       string `json:"installed_version,omitempty"`
	AttemptedVersion       string `json:"attempted_version,omitempty"`
	Status                 string `json:"status"`
	ErrorCode              string `json:"error_code,omitempty"`
	Message                string `json:"message"`
	TransactionID          string `json:"transaction_id,omitempty"`
	InstallerPID           int    `json:"installer_pid,omitempty"`
	ReceiptPath            string `json:"receipt_path,omitempty"`
	ExecutablePath         string `json:"executable_path,omitempty"`
	PreviousExecutablePath string `json:"previous_executable_path,omitempty"`
	MigrationStatus        string `json:"migration_status,omitempty"`
	MigrationMessage       string `json:"migration_message,omitempty"`
	ReceiptRestarted       bool   `json:"receipt_restarted,omitempty"`
	UpdatedAt              string `json:"updated_at"`
	ReceiptCompletedAt     string `json:"receipt_completed_at,omitempty"`
}

type State struct {
	SchemaVersion string                   `json:"schema_version"`
	Products      map[string]ProductRecord `json:"products"`
	UpdatedAt     string                   `json:"updated_at"`
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

func Load(path string) (State, error) {
	if !filepath.IsAbs(path) {
		return State{}, errors.New("更新状态路径必须是绝对路径")
	}
	data, err := readRegularFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("更新状态 JSON 无效: %w", err)
	}
	if state.SchemaVersion != StateSchemaVersion {
		return State{}, errors.New("更新状态 schema_version 不受支持")
	}
	if state.Products == nil {
		state.Products = map[string]ProductRecord{}
	}
	for key, record := range state.Products {
		if key != record.ProductID || !productIDPattern.MatchString(key) {
			return State{}, errors.New("更新状态包含无效的产品身份")
		}
	}
	return state, nil
}

func RecordAttempt(path string, record ProductRecord) (State, error) {
	if !productIDPattern.MatchString(record.ProductID) || strings.TrimSpace(record.AttemptedVersion) == "" || strings.TrimSpace(record.TransactionID) == "" {
		return State{}, errors.New("更新尝试缺少产品、版本或事务身份")
	}
	if !filepath.IsAbs(record.ReceiptPath) {
		return State{}, errors.New("安装回执路径必须是绝对路径")
	}
	if record.ExecutablePath != "" && !filepath.IsAbs(record.ExecutablePath) {
		return State{}, errors.New("目标应用路径必须是绝对路径")
	}
	if record.PreviousExecutablePath != "" && !filepath.IsAbs(record.PreviousExecutablePath) {
		return State{}, errors.New("迁移前应用路径必须是绝对路径")
	}
	state, err := Load(path)
	if err != nil {
		return State{}, err
	}
	previous := state.Products[record.ProductID]
	record.InstalledVersion = previous.InstalledVersion
	if record.ExecutablePath == "" {
		record.ExecutablePath = previous.ExecutablePath
	}
	if record.PreviousExecutablePath != "" && !strings.EqualFold(filepath.Clean(record.PreviousExecutablePath), filepath.Clean(record.ExecutablePath)) {
		record.MigrationStatus = "pending"
		record.MigrationMessage = "等待规范目录安装成功并重启后清理旧版程序文件"
	} else if previous.MigrationStatus == "pending" || previous.MigrationStatus == "cleanup_failed" {
		record.PreviousExecutablePath = previous.PreviousExecutablePath
		record.MigrationStatus = previous.MigrationStatus
		record.MigrationMessage = previous.MigrationMessage
	}
	record.ReceiptRestarted = false
	record.Status = "starting"
	record.Message = "Launcher 正在启动一次性安装程序"
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.Products[record.ProductID] = record
	return state, save(path, state)
}

func RecordInstallerStarted(path, productID, transactionID string, pid int, status string) (State, error) {
	state, err := Load(path)
	if err != nil {
		return State{}, err
	}
	record, ok := state.Products[productID]
	if !ok || record.TransactionID != transactionID || pid <= 0 {
		return State{}, errors.New("找不到对应的更新事务")
	}
	if status != "installer_started" && status != "self_update_installer_started" {
		return State{}, errors.New("安装程序启动状态无效")
	}
	record.Status = status
	record.InstallerPID = pid
	record.Message = "一次性安装程序已启动，等待安装回执"
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.Products[productID] = record
	return state, save(path, state)
}

func RecordAttemptFailed(path, productID, transactionID, errorCode, message string) (State, error) {
	state, err := Load(path)
	if err != nil {
		return State{}, err
	}
	record, ok := state.Products[productID]
	if !ok || record.TransactionID != transactionID {
		return State{}, errors.New("找不到对应的更新事务")
	}
	record.Status = "install_start_failed"
	record.ErrorCode = strings.TrimSpace(errorCode)
	record.Message = strings.TrimSpace(message)
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.Products[productID] = record
	return state, save(path, state)
}

func RecordMigrationCleanup(path, productID, transactionID, status, message string) (State, error) {
	if status != "completed" && status != "cleanup_failed" {
		return State{}, errors.New("迁移清理状态无效")
	}
	state, err := Load(path)
	if err != nil {
		return State{}, err
	}
	record, ok := state.Products[productID]
	if !ok || record.TransactionID != transactionID || record.MigrationStatus == "" {
		return State{}, errors.New("找不到对应的目录迁移事务")
	}
	record.MigrationStatus = status
	record.MigrationMessage = strings.TrimSpace(message)
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.Products[productID] = record
	return state, save(path, state)
}

func RemoveProduct(path, productID string) (State, error) {
	if !productIDPattern.MatchString(productID) {
		return State{}, errors.New("产品身份无效")
	}
	state, err := Load(path)
	if err != nil {
		return State{}, err
	}
	delete(state.Products, productID)
	return state, save(path, state)
}

func ReconcilePending(path string) (State, error) {
	state, err := Load(path)
	if err != nil {
		return State{}, err
	}
	changed := false
	for productID, record := range state.Products {
		if record.Status != "starting" && record.Status != "installer_started" && record.Status != "self_update_installer_started" {
			continue
		}
		receipt, readErr := loadReceipt(record.ReceiptPath)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			record.Status = "receipt_invalid"
			record.ErrorCode = "RECEIPT_INVALID"
			record.Message = readErr.Error()
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			state.Products[productID] = record
			changed = true
			continue
		}
		if receipt.ProductID != record.ProductID || receipt.Version != record.AttemptedVersion || receipt.TransactionID != record.TransactionID {
			record.Status = "receipt_invalid"
			record.ErrorCode = "RECEIPT_IDENTITY_MISMATCH"
			record.Message = "安装回执与 Launcher 记录的产品、版本或事务不一致"
		} else if receipt.ExecutablePath != "" && (record.ExecutablePath == "" || !strings.EqualFold(filepath.Clean(receipt.ExecutablePath), filepath.Clean(record.ExecutablePath))) {
			if !adoptLegacyLauncherMigration(&record, receipt) {
				record.Status = "receipt_invalid"
				record.ErrorCode = "RECEIPT_TARGET_MISMATCH"
				record.Message = "安装回执中的应用路径不在 Launcher 预先登记的目标上"
			} else {
				applyReceipt(&record, receipt)
			}
		} else {
			applyReceipt(&record, receipt)
		}
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		state.Products[productID] = record
		changed = true
	}
	if !changed {
		return state, nil
	}
	return state, save(path, state)
}

func adoptLegacyLauncherMigration(record *ProductRecord, receipt Receipt) bool {
	if record.ProductID != "meow-launcher" || record.ExecutablePath == "" || receipt.ExecutablePath == "" {
		return false
	}
	legacyExecutable := filepath.Clean(record.ExecutablePath)
	canonicalExecutable := filepath.Clean(receipt.ExecutablePath)
	if installlayout.IsProductDirectory(filepath.Dir(legacyExecutable), record.ProductID) ||
		!installlayout.IsProductDirectory(filepath.Dir(canonicalExecutable), record.ProductID) ||
		!strings.EqualFold(filepath.Base(canonicalExecutable), "Meow Launcher.exe") ||
		!strings.EqualFold(filepath.VolumeName(legacyExecutable), filepath.VolumeName(canonicalExecutable)) {
		return false
	}
	if receipt.Status == "failed" {
		// The canonical transaction rolled back and the old installation remains
		// authoritative. Accept the failure receipt without changing its path.
		return true
	}
	if receipt.Status != "installed" && receipt.Status != "installed_restart_failed" {
		return false
	}
	record.PreviousExecutablePath = legacyExecutable
	record.ExecutablePath = canonicalExecutable
	record.MigrationStatus = "pending"
	record.MigrationMessage = "规范目录安装已完成；等待新版启动确认后清理旧版程序文件"
	return true
}

func applyReceipt(record *ProductRecord, receipt Receipt) {
	record.Status = receipt.Status
	record.ErrorCode = receipt.ErrorCode
	record.Message = receipt.Message
	record.ReceiptCompletedAt = receipt.CompletedAt
	record.ReceiptRestarted = receipt.Restarted
	if receipt.Status == "installed" || receipt.Status == "installed_restart_failed" {
		record.InstalledVersion = receipt.Version
	}
}

func loadReceipt(path string) (Receipt, error) {
	if !filepath.IsAbs(path) {
		return Receipt{}, errors.New("安装回执路径不是绝对路径")
	}
	data, err := readRegularFile(path)
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return Receipt{}, errors.New("安装回执 JSON 无效")
	}
	if receipt.SchemaVersion != ReceiptSchemaVersion || !productIDPattern.MatchString(receipt.ProductID) || strings.TrimSpace(receipt.TransactionID) == "" || strings.TrimSpace(receipt.Version) == "" {
		return Receipt{}, errors.New("安装回执身份或 schema_version 无效")
	}
	if receipt.Status != "installed" && receipt.Status != "installed_restart_failed" && receipt.Status != "failed" {
		return Receipt{}, errors.New("安装回执状态无效")
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.CompletedAt); err != nil {
		return Receipt{}, errors.New("安装回执完成时间无效")
	}
	if receipt.ExecutablePath != "" && !filepath.IsAbs(receipt.ExecutablePath) {
		return Receipt{}, errors.New("安装回执应用路径不是绝对路径")
	}
	return receipt, nil
}

func emptyState() State {
	return State{SchemaVersion: StateSchemaVersion, Products: map[string]ProductRecord{}}
}

func save(path string, state State) error {
	state.SchemaVersion = StateSchemaVersion
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if state.Products == nil {
		state.Products = map[string]ProductRecord{}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("更新状态或回执不是普通文件")
	}
	return os.ReadFile(filepath.Clean(path))
}

func atomicWrite(path string, data []byte) error {
	if !filepath.IsAbs(path) {
		return errors.New("更新状态路径必须是绝对路径")
	}
	directory := filepath.Dir(filepath.Clean(path))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".meow-update-state-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
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
			return errors.New("已有更新状态不是普通文件")
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
