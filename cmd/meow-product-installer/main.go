package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"meow-launcher/internal/installerui"
	"meow-launcher/internal/installlayout"
	"meow-launcher/internal/productinstaller"
)

func main() {
	productID := flag.String("meow-product", "", "product identity supplied by Launcher")
	version := flag.String("meow-version", "", "target version supplied by Launcher")
	transactionID := flag.String("meow-transaction", "", "one-time update transaction identity")
	receiptPath := flag.String("meow-receipt", "", "absolute install receipt path")
	waitPID := flag.Int("meow-wait-pid", 0, "Launcher PID that must exit before self-update")
	returnAction := flag.String("meow-return", "", "post-install action")
	targetExecutable := flag.String("meow-target-executable", "", "absolute target executable")
	restartLauncher := flag.Bool("meow-restart-launcher", false, "restart Launcher after self-update")
	initialDirectory := flag.String("meow-initial-directory", "", "explicit directory for a non-interactive first Launcher installation")
	flag.Parse()
	interactive := flag.NFlag() == 0 && flag.NArg() == 0
	executable, err := os.Executable()
	if err != nil {
		fail(err, 2, interactive)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if *initialDirectory != "" {
		if flag.NFlag() != 1 || flag.NArg() != 0 {
			fail(fmt.Errorf("Launcher 首次安装目录参数不能与更新参数混用"), 2, false)
		}
		directory, layoutErr := resolveInitialDirectory(*initialDirectory)
		if layoutErr != nil {
			fail(layoutErr, 2, false)
		}
		if makeErr := os.MkdirAll(directory, 0o700); makeErr != nil {
			fail(makeErr, 1, false)
		}
		if _, installErr := productinstaller.InstallInitialLauncher(ctx, executable, directory, productinstaller.Options{}); installErr != nil {
			fail(installErr, 1, false)
		}
		return
	}
	if flag.NFlag() == 0 && flag.NArg() == 0 {
		metadata, inspectErr := productinstaller.InspectInitialLauncherBundle(executable)
		if inspectErr != nil {
			fail(inspectErr, 2, true)
		}
		selection, selected, chooseErr := installerui.ChooseFolder("选择 Meow Launcher 的安装磁盘")
		if chooseErr != nil {
			fail(chooseErr, 1, true)
		}
		if !selected {
			return
		}
		directory, layoutErr := installlayout.DirectoryFromSelection(selection, "meow-launcher")
		if layoutErr != nil {
			fail(layoutErr, 1, true)
		}
		target := filepath.Join(directory, filepath.FromSlash(metadata.ExecutableRelativePath))
		if !installerui.Confirm("将 Meow Launcher "+metadata.Version+" 安装到：\n\n"+target+"\n\n以后更新会继续使用这个位置。", "确认安装") {
			return
		}
		if makeErr := os.MkdirAll(directory, 0o700); makeErr != nil {
			fail(makeErr, 1, true)
		}
		if _, installErr := productinstaller.InstallInitialLauncher(ctx, executable, directory, productinstaller.Options{}); installErr != nil {
			fail(installErr, 1, true)
		}
		installerui.ShowInfo("Meow Launcher 已安装并启动。安装程序现在会退出，不会常驻后台。", "安装完成")
		return
	}
	_, err = productinstaller.Install(ctx, executable, productinstaller.Invocation{
		ProductID: *productID, Version: *version, TransactionID: *transactionID, ReceiptPath: *receiptPath,
		WaitPID: *waitPID, ReturnAction: *returnAction, TargetExecutable: *targetExecutable, RestartLauncher: *restartLauncher,
	}, productinstaller.Options{})
	if err != nil {
		fail(err, 1, false)
	}
}

func resolveInitialDirectory(selection string) (string, error) {
	if strings.TrimSpace(os.Getenv("MEOW_DESKTOP_SHORTCUT_TEST_ROOT")) == "" {
		return installlayout.DirectoryFromSelection(selection, "meow-launcher")
	}
	machineRoot := filepath.Clean(strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_MACHINE_ROOT")))
	directory := filepath.Clean(strings.TrimSpace(selection))
	if !filepath.IsAbs(machineRoot) || !filepath.IsAbs(directory) || !installlayout.IsProductDirectory(directory, "meow-launcher") {
		return "", fmt.Errorf("隔离首次安装目录必须是 MEOW_LAUNCHER_MACHINE_ROOT 内的 Meow System\\Launcher")
	}
	relative, err := filepath.Rel(machineRoot, directory)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("隔离首次安装目录超出 MEOW_LAUNCHER_MACHINE_ROOT")
	}
	return directory, nil
}

func fail(err error, code int, showUI bool) {
	fmt.Fprintln(os.Stderr, err)
	if showUI {
		installerui.ShowError(err.Error(), "Meow 安装程序")
	}
	os.Exit(code)
}
