package updateinstall

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"meow-launcher/internal/updatecontract"
)

type ExecuteRequest struct {
	Release          updatecontract.ProductRelease
	InstallerPath    string
	TransactionID    string
	ReceiptPath      string
	ReturnAction     string
	TargetExecutable string
	LauncherPID      int
}

type ExecuteResult struct {
	Status string   `json:"status"`
	PID    int      `json:"pid"`
	Args   []string `json:"args"`
}

type InstallerStarter func(context.Context, string, []string) (int, error)

type Executor struct {
	VerifySigner SignatureVerifier
	Start        InstallerStarter
}

func (e Executor) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	if err := request.Release.Validate(); err != nil {
		return ExecuteResult{}, newError("RELEASE_INVALID", "发布记录无效，不能执行安装", err)
	}
	if !filepath.IsAbs(request.InstallerPath) {
		return ExecuteResult{}, newError("INSTALLER_PATH_INVALID", "安装程序路径必须是绝对路径", nil)
	}
	info, err := os.Lstat(filepath.Clean(request.InstallerPath))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ExecuteResult{}, newError("INSTALLER_PATH_INVALID", "已下载的安装程序不存在或不是普通文件", err)
	}
	if err := updatecontract.VerifyFileSHA256(request.InstallerPath, request.Release.SHA256); err != nil {
		return ExecuteResult{}, newError("HASH_MISMATCH", "HASH_MISMATCH：安装前复核发现文件已变化", err)
	}
	verifier := e.VerifySigner
	if verifier == nil {
		verifier = PlatformSignatureVerifier{}
	}
	if err := verifier.Verify(ctx, request.InstallerPath, request.Release.Signature); err != nil {
		return ExecuteResult{}, err
	}
	args, err := installerArguments(request)
	if err != nil {
		return ExecuteResult{}, err
	}
	starter := e.Start
	if starter == nil {
		starter = startInstaller
	}
	pid, err := starter(ctx, request.InstallerPath, args)
	if err != nil {
		return ExecuteResult{}, newError("INSTALLER_START_FAILED", "无法启动一次性安装程序", err)
	}
	status := "installer_started"
	if request.Release.ProductID == "meow-launcher" {
		status = "self_update_installer_started"
	}
	return ExecuteResult{Status: status, PID: pid, Args: args}, nil
}

func installerArguments(request ExecuteRequest) ([]string, error) {
	installerType := strings.ToLower(strings.TrimSpace(request.Release.InstallerType))
	standardInstaller := installerType == "nsis" || installerType == "meow-bundle"
	oneShotInstaller := installerType == "nsis-one-shot" || installerType == "meow-bundle-one-shot"
	if !standardInstaller && !oneShotInstaller {
		return nil, newError("INSTALLER_TYPE_UNSUPPORTED", "Launcher 不支持这个 installer_type", nil)
	}
	if request.Release.ProductID != "meow-launcher" && oneShotInstaller {
		return nil, newError("INSTALLER_TYPE_UNSUPPORTED", "普通产品不能使用 Launcher 自更新安装包", nil)
	}
	if strings.TrimSpace(request.TransactionID) == "" || !filepath.IsAbs(request.ReceiptPath) {
		return nil, newError("INSTALL_TRANSACTION_INVALID", "安装事务号或回执路径无效", nil)
	}
	args := []string{
		"--meow-product", request.Release.ProductID,
		"--meow-version", request.Release.Version,
		"--meow-transaction", request.TransactionID,
		"--meow-receipt", filepath.Clean(request.ReceiptPath),
	}
	if request.ReturnAction == "restart" {
		args = append(args, "--meow-return", "restart")
	}
	if request.TargetExecutable != "" {
		if !filepath.IsAbs(request.TargetExecutable) {
			return nil, newError("INSTALL_TARGET_INVALID", "目标应用路径必须是绝对路径", nil)
		}
		args = append(args, "--meow-target-executable", filepath.Clean(request.TargetExecutable))
	}
	if request.Release.ProductID == "meow-launcher" {
		if !oneShotInstaller || request.LauncherPID <= 0 {
			return nil, newError("SELF_UPDATE_INVALID", "Launcher 自更新必须使用一次性安装程序并等待当前进程退出", nil)
		}
		args = append(args, "--meow-wait-pid", strconv.Itoa(request.LauncherPID), "--meow-restart-launcher")
	}
	return args, nil
}

func startInstaller(_ context.Context, path string, args []string) (int, error) {
	// The installer owns a transaction that intentionally outlives this short
	// Launcher RPC. Binding it to the RPC context would kill it as soon as the
	// start call returns and its context is cancelled.
	command := exec.Command(path, args...)
	command.Dir = filepath.Dir(path)
	if err := command.Start(); err != nil {
		return 0, err
	}
	if command.Process == nil {
		return 0, errors.New("installer process missing")
	}
	pid := command.Process.Pid
	if err := command.Process.Release(); err != nil {
		return 0, err
	}
	return pid, nil
}
