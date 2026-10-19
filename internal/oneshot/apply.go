package oneshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type ApplyOptions struct {
	WaitForPID   func(context.Context, int) error
	BeforeCommit func(int, FileEntry) error
	AfterCommit  func() error
	Restart      func(string, []string) error
}

type preparedFile struct {
	entry       FileEntry
	temporary   string
	backup      string
	hadExisting bool
	committed   bool
}

type preparedRemoval struct {
	target    string
	backup    string
	committed bool
}

func Apply(ctx context.Context, plan Plan, options ApplyOptions) (Receipt, error) {
	if err := plan.Validate(); err != nil {
		return failureReceipt(plan, err), err
	}
	waiter := options.WaitForPID
	if waiter == nil {
		waiter = WaitForPID
	}
	for _, pid := range plan.WaitPIDs {
		if pid <= 0 {
			return fail(plan, installError("PLAN_INVALID", "等待 PID 无效", nil))
		}
		if err := waiter(ctx, pid); err != nil {
			return fail(plan, installError("PROCESS_STILL_RUNNING", "等待应用退出失败", err))
		}
	}
	if err := os.MkdirAll(plan.BackupRoot, 0o700); err != nil {
		return fail(plan, installError("BACKUP_FAILED", "无法创建更新备份目录", err))
	}
	prepared := make([]preparedFile, 0, len(plan.Files))
	for index, entry := range plan.Files {
		if err := VerifyFile(entry.SourcePath, entry.SHA256); err != nil {
			cleanupPrepared(prepared)
			return fail(plan, err)
		}
		if err := os.MkdirAll(filepath.Dir(entry.TargetPath), 0o755); err != nil {
			cleanupPrepared(prepared)
			return fail(plan, installError("INSTALL_FAILED", "无法创建应用目录", err))
		}
		temporary, err := copyToTemporary(entry.SourcePath, filepath.Dir(entry.TargetPath), plan.TransactionID)
		if err != nil {
			cleanupPrepared(prepared)
			return fail(plan, installError("INSTALL_FAILED", "无法准备新应用文件", err))
		}
		if err := VerifyFile(temporary, entry.SHA256); err != nil {
			_ = os.Remove(temporary)
			cleanupPrepared(prepared)
			return fail(plan, installError("HASH_MISMATCH", "安装载荷复制到目标磁盘后校验失败", err))
		}
		item := preparedFile{entry: entry, temporary: temporary, backup: filepath.Join(plan.BackupRoot, fmt.Sprintf("%04d.bak", index))}
		if info, err := os.Lstat(entry.TargetPath); err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				os.Remove(temporary)
				cleanupPrepared(prepared)
				return fail(plan, installError("INSTALL_TARGET_INVALID", "目标应用文件不是普通文件", nil))
			}
			item.hadExisting = true
		} else if !errors.Is(err, os.ErrNotExist) {
			os.Remove(temporary)
			cleanupPrepared(prepared)
			return fail(plan, installError("INSTALL_TARGET_INVALID", "无法检查目标应用文件", err))
		}
		prepared = append(prepared, item)
	}
	removals := make([]preparedRemoval, 0, len(plan.RemovePaths))
	for index, target := range plan.RemovePaths {
		info, statErr := os.Lstat(target)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			cleanupPrepared(prepared)
			return fail(plan, installError("INSTALL_TARGET_INVALID", "待移除的旧应用文件不是普通文件", statErr))
		}
		removals = append(removals, preparedRemoval{target: target, backup: filepath.Join(plan.BackupRoot, fmt.Sprintf("remove-%04d.bak", index))})
	}

	for index := range prepared {
		item := &prepared[index]
		if options.BeforeCommit != nil {
			if err := options.BeforeCommit(index, item.entry); err != nil {
				rollback(prepared, removals)
				return fail(plan, installError("INSTALL_FAILED", "安装事务被中止并已恢复旧版本", err))
			}
		}
		if item.hadExisting {
			_ = os.Remove(item.backup)
			if err := os.Rename(item.entry.TargetPath, item.backup); err != nil {
				rollback(prepared, removals)
				return fail(plan, installError("TARGET_IN_USE_OR_DENIED", "无法备份旧应用文件；请关闭目标应用后重试，旧版本未改变", err))
			}
		}
		if err := os.Rename(item.temporary, item.entry.TargetPath); err != nil {
			if item.hadExisting {
				_ = os.Rename(item.backup, item.entry.TargetPath)
			}
			rollback(prepared, removals)
			return fail(plan, installError("INSTALL_FAILED", "无法替换应用文件；旧版本已恢复", err))
		}
		item.committed = true
	}
	for index := range removals {
		item := &removals[index]
		_ = os.Remove(item.backup)
		if err := os.Rename(item.target, item.backup); err != nil {
			rollback(prepared, removals)
			return fail(plan, installError("TARGET_IN_USE_OR_DENIED", "无法移除已登记的旧应用文件；旧版本已恢复", err))
		}
		item.committed = true
	}
	if options.AfterCommit != nil {
		if err := options.AfterCommit(); err != nil {
			rollback(prepared, removals)
			return fail(plan, installError("INSTALL_FAILED", "安装后的系统入口创建失败；应用文件已恢复到安装前状态", err))
		}
	}

	receipt := Receipt{TransactionID: plan.TransactionID, ProductID: plan.ProductID, Version: plan.Version, Status: "installed", Message: "应用文件已更新并保留回滚备份", ExecutablePath: plan.RestartExecutable}
	if plan.RestartExecutable != "" {
		restart := options.Restart
		if restart == nil {
			restart = startProcess
		}
		if err := restart(plan.RestartExecutable, plan.RestartArguments); err != nil {
			receipt.Status = "installed_restart_failed"
			receipt.ErrorCode = "RESTART_FAILED"
			receipt.Message = "应用已更新，但重新启动失败"
		} else {
			receipt.Restarted = true
		}
	}
	if err := WriteReceipt(plan.ReceiptPath, receipt); err != nil {
		return receipt, installError("RECEIPT_FAILED", "应用已更新，但安装回执写入失败", err)
	}
	return receipt, nil
}

func fail(plan Plan, err error) (Receipt, error) {
	receipt := failureReceipt(plan, err)
	_ = WriteReceipt(plan.ReceiptPath, receipt)
	return receipt, err
}

func failureReceipt(plan Plan, err error) Receipt {
	return Receipt{TransactionID: plan.TransactionID, ProductID: plan.ProductID, Version: plan.Version, Status: "failed", ErrorCode: CodeOf(err), Message: err.Error(), ExecutablePath: plan.RestartExecutable}
}

func rollback(files []preparedFile, removals []preparedRemoval) {
	for index := len(removals) - 1; index >= 0; index-- {
		item := removals[index]
		if item.committed {
			_ = os.Rename(item.backup, item.target)
		}
	}
	for index := len(files) - 1; index >= 0; index-- {
		item := files[index]
		if item.committed {
			_ = os.Remove(item.entry.TargetPath)
			if item.hadExisting {
				_ = os.Rename(item.backup, item.entry.TargetPath)
			}
		} else {
			_ = os.Remove(item.temporary)
		}
	}
}

func cleanupPrepared(files []preparedFile) {
	for _, item := range files {
		_ = os.Remove(item.temporary)
	}
}

func copyToTemporary(source, targetDirectory, transactionID string) (string, error) {
	sourceFile, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer sourceFile.Close()
	temporary, err := os.CreateTemp(targetDirectory, ".meow-update-"+transactionID+"-*.tmp")
	if err != nil {
		return "", err
	}
	path := temporary.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if _, err = io.Copy(temporary, sourceFile); err != nil {
		temporary.Close()
		return "", err
	}
	if err = temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	if err = temporary.Close(); err != nil {
		return "", err
	}
	return path, nil
}

func startProcess(executable string, arguments []string) error {
	command := exec.Command(executable, arguments...)
	command.Dir = filepath.Dir(executable)
	return command.Start()
}

func WaitForPID(ctx context.Context, pid int) error {
	deadline := time.NewTicker(100 * time.Millisecond)
	defer deadline.Stop()
	for {
		running, err := processRunning(pid)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
		}
	}
}
