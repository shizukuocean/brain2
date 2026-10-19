package agentconnect

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"

	"meow-launcher/internal/brain2"
	"meow-launcher/internal/settings"
)

const (
	serverName     = "brain2-meow"
	MCPHostVersion = brain2.MCPHostVersion
)

var legacyManagedSkillHashes = map[string]struct{}{
	// brain2-connect bundled with MCP Host v0.6.1. Keeping the exact hash lets
	// the first manifest-aware release upgrade known Launcher content while
	// still rejecting any user-edited variant.
	"3c2ec79bd25b40aebd4b8bda4e2e3cfe4462b0b6af54557653c1f257b3040240": {},
}

// The native Windows and Linux hosts are built from cmd/brain2-mcp-host before
// packaging. Keeping the MCP process separate from the Wails GUI guarantees
// ordinary stdin/stdout behavior and gives the connector its own lifecycle.
//
//go:embed assets/brain2-mcp-host-windows-amd64.exe assets/brain2-mcp-host-linux-amd64
var runtimeAssets embed.FS

type Detection struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Surface           string   `json:"surface"`
	Detected          bool     `json:"detected"`
	QuickConnect      bool     `json:"quick_connect"`
	PromptConnect     bool     `json:"prompt_connect"`
	Executable        string   `json:"executable"`
	Version           string   `json:"version"`
	Status            string   `json:"status"`
	Message           string   `json:"message"`
	MCPStatus         string   `json:"mcp_status"`
	MCPMessage        string   `json:"mcp_message"`
	MCPVersion        string   `json:"mcp_version"`
	MCPAvailable      string   `json:"mcp_available_version"`
	RollbackAvailable bool     `json:"rollback_available"`
	PrefixArgs        []string `json:"-"`
}

type Result struct {
	AgentID            string   `json:"agent_id"`
	Status             string   `json:"status"`
	ConnectionID       string   `json:"connection_id"`
	Message            string   `json:"message"`
	Prompt             string   `json:"prompt"`
	ReceiptPath        string   `json:"receipt_path"`
	ChangedPaths       []string `json:"changed_paths"`
	RestartRequired    bool     `json:"restart_required"`
	SkillUpdated       bool     `json:"skill_updated"`
	HostUpdated        bool     `json:"host_updated"`
	ConfigUpdated      bool     `json:"config_updated"`
	SkillConflict      bool     `json:"skill_conflict"`
	VerificationPrompt string   `json:"verification_prompt"`
}

type skillManifest struct {
	SchemaVersion    string `json:"schema_version"`
	AgentID          string `json:"agent_id"`
	Path             string `json:"path"`
	ManagedSHA256    string `json:"managed_sha256"`
	InstalledVersion string `json:"installed_version"`
	UpdatedAt        string `json:"updated_at"`
}

type skillInstallResult struct {
	ChangedPaths []string
	Updated      bool
	Conflict     bool
	Path         string
}

type Progress struct {
	AgentID string `json:"agent_id"`
	Phase   string `json:"phase"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
}

type ProgressReporter func(Progress)

type ConnectorBackup struct {
	SchemaVersion  string   `json:"schema_version"`
	AgentID        string   `json:"agent_id"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	Status         string   `json:"status"`
	RegistryPath   string   `json:"registry_path"`
	PreviousHost   string   `json:"previous_host,omitempty"`
	PreviousConfig string   `json:"previous_config"`
	ChangedPaths   []string `json:"changed_paths,omitempty"`
}

type commandRunner func(context.Context, string, []string, string) ([]byte, error)

type PromptOptions struct {
	DeviceID    string
	WorkIntent  string
	ProjectID   string
	ProjectName string
	ProjectGoal string
}

func DetectAll(ctx context.Context) []Detection {
	codexPath, codexPrefix := findCodexCommand()
	codexDesktopPath := findCodexDesktopCommand()
	claudePath := findClaudeCommand()
	hermesPath, hermesPrefix, hermesEnvironment := findHermesCommand(ctx)
	claudeDesktopPath := findExisting([]string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Claude", "Claude.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "AnthropicClaude", "Claude.exe"),
	})

	items := []Detection{
		detectionFor(ctx, "codex", "Codex CLI", "cli", codexPath, codexPrefix, true, true),
		detectionFor(ctx, "claude-code", "Claude Code", "cli", claudePath, nil, true, true),
		detectionFor(ctx, "hermes", "Hermes", "cli", hermesPath, hermesPrefix, true, true),
		desktopDetection("codex-desktop", "Codex Desktop", codexDesktopPath),
		{ID: "claude-desktop", Name: "Claude Desktop", Surface: "desktop", Detected: claudeDesktopPath != "", QuickConnect: true, PromptConnect: true, Executable: claudeDesktopPath},
	}
	for i := range items {
		if items[i].ID == "codex-desktop" && items[i].Detected {
			items[i].Message = "已检测到 Codex Desktop · 可安全写入用户配置"
		}
		if items[i].ID == "hermes" && items[i].Detected && hermesEnvironment != "" {
			items[i].Message = "可快速连接 · WSL " + hermesEnvironment
		}
		if items[i].ID == "claude-desktop" {
			if items[i].Detected {
				items[i].Status = "installable"
				items[i].Message = "已检测到 Claude Desktop；可生成 MCPB 扩展并用 Prompt 激活"
			} else {
				items[i].Status = "not_detected"
				items[i].Message = "未检测到 Claude Desktop"
			}
		}
	}
	return items
}

func sameExecutable(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

// DetectAllForWorkspace augments ordinary Agent discovery with a read-only
// audit of the persistent brain2-meow registration. It never changes an Agent
// configuration; callers may use MCPStatus=update_available to offer the
// explicit managed upgrade path. Detection never updates automatically.
func DetectAllForWorkspace(ctx context.Context, registryPath, launcherExe string) []Detection {
	items := DetectAll(ctx)
	windowsToken := strings.TrimSuffix(versionedHostName(), ".exe")
	linuxToken := versionedLinuxHostName()
	for i := range items {
		if !items[i].Detected || items[i].Executable == "" || items[i].ID == "claude-desktop" {
			continue
		}
		token := windowsToken
		if isWSLDetection(items[i]) {
			token = linuxToken
		}
		agentRegistry := registryPath
		if isWSLDetection(items[i]) {
			var convertErr error
			agentRegistry, convertErr = windowsPathToWSL(registryPath)
			if convertErr != nil {
				items[i].MCPStatus, items[i].MCPMessage = "unknown", convertErr.Error()
				continue
			}
		}
		items[i].MCPStatus, items[i].MCPMessage, items[i].MCPVersion = auditMCPRegistration(ctx, runCommand, items[i], agentRegistry, token)
		items[i].MCPAvailable = MCPHostVersion
		items[i].RollbackAvailable = connectorRollbackAvailable(items[i].ID)
	}
	return items
}

func auditMCPRegistration(ctx context.Context, runner commandRunner, detection Detection, registryPath, expectedHostToken string) (string, string, string) {
	auditCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var output []byte
	var err error
	switch detection.ID {
	case "codex":
		output, err = runner(auditCtx, detection.Executable, withPrefix(detection, "mcp", "get", serverName, "--json"), "")
	case "codex-desktop":
		output, err = readCodexDesktopMCPRegistration()
	case "claude-code":
		output, err = runner(auditCtx, detection.Executable, []string{"mcp", "get", serverName}, "")
	case "hermes":
		output, err = readHermesConfig(auditCtx, runner, detection)
	default:
		return "unknown", "该 Agent 尚无托管 MCP 审计器", ""
	}
	if err != nil || !strings.Contains(strings.ToLower(string(output)), serverName) {
		return "not_registered", "尚未发现持久化 brain2-meow 配置", ""
	}
	if configUsesRegistry(output) && !configContainsPath(output, registryPath) {
		return "conflict", "该 Agent 使用另一套 Launcher 记忆库注册表；不会自动覆盖", configuredHostVersion(output)
	}
	currentVersion := configuredHostVersion(output)
	if !configUsesRegistry(output) {
		return "update_available", "当前是旧版单目录连接器；迁移一次后，新增记忆库无需重复配置", currentVersion
	}
	if expectedHostToken != "" && strings.Contains(strings.ToLower(string(output)), strings.ToLower(expectedHostToken)) && configContainsPath(output, registryPath) {
		return "current", "全局 Brain2 连接器已就绪；新增记忆库无需重复配置", MCPHostVersion
	}
	return "update_available", "Brain2 连接器有可选更新", currentVersion
}

func configuredHostVersion(output []byte) string {
	lower := strings.ToLower(string(output))
	marker := "brain2-mcp-host-v"
	index := strings.Index(lower, marker)
	if index < 0 {
		return "legacy"
	}
	version := lower[index+len(marker):]
	if end := strings.IndexAny(version, `\\/\"' ,]}\r\n\t`); end >= 0 {
		version = version[:end]
	}
	version = strings.TrimSuffix(version, ".exe")
	if version == "" {
		return "legacy"
	}
	return version
}

func readHermesConfig(ctx context.Context, runner commandRunner, detection Detection) ([]byte, error) {
	if isWSLDetection(detection) {
		return runner(ctx, detection.Executable, append(wslBaseArgs(detection), "sh", "-lc", `cat "$HOME/.hermes/config.yaml"`), "")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(home, ".hermes", "config.yaml"))
}

func PreparePrompt(root, registryPath, agentID, launcherExe, skillContent string, options PromptOptions) (Result, error) {
	definition, ok := brain2.AgentDefinitionByID(agentID)
	if !ok {
		return Result{}, fmt.Errorf("不支持的 Agent %q", agentID)
	}
	deviceID := strings.TrimSpace(options.DeviceID)
	if deviceID == "" {
		deviceID = "A"
	}
	localReady, err := isLocallyRegisteredForDevice(root, deviceID, agentID)
	if err != nil {
		return Result{}, err
	}
	if err := validatePromptOptions(root, &options); err != nil {
		return Result{}, err
	}
	skillSource, err := preparePromptSkillSource(root, skillContent)
	if err != nil {
		return Result{}, err
	}
	hostCommand := launcherExe
	if definition.ID == "codex" || definition.ID == "codex-desktop" || definition.ID == "claude-code" {
		var hostErr error
		hostCommand, _, hostErr = installStableHost()
		if hostErr != nil {
			return Result{}, fmt.Errorf("准备稳定 Brain2 Host 失败: %w", hostErr)
		}
	}
	installGuide := manualInstallGuide(definition, registryPath, hostCommand, skillSource)
	task, prompt, err := brain2.PrepareConnectionWithOptions(root, agentID, "prompt", brain2.ConnectionPromptOptions{
		DeviceID: options.DeviceID, LocalReady: localReady, WorkIntent: options.WorkIntent, ProjectID: options.ProjectID,
		ProjectName: options.ProjectName, ProjectGoal: options.ProjectGoal,
		SkillSource: skillSource, InstallGuide: installGuide,
	})
	if err != nil {
		return Result{}, err
	}
	message := "完整接入 Prompt 已生成"
	if localReady {
		message = "开始工作 Prompt 已生成"
	}
	return Result{
		AgentID: agentID, Status: string(brain2.ConnectionPrepared),
		ConnectionID: task.ConnectionID, Message: message,
		Prompt: prompt,
	}, nil
}

func validatePromptOptions(root string, options *PromptOptions) error {
	options.WorkIntent = strings.TrimSpace(options.WorkIntent)
	options.ProjectID = strings.TrimSpace(options.ProjectID)
	options.ProjectName = strings.TrimSpace(options.ProjectName)
	options.ProjectGoal = strings.TrimSpace(options.ProjectGoal)
	if options.WorkIntent == "" {
		options.WorkIntent = "brain"
	}
	switch options.WorkIntent {
	case "brain":
		return nil
	case "new_project":
		return nil
	case "existing_project":
		if options.ProjectID == "" {
			return errors.New("请选择要进入的已有项目")
		}
		projects, err := brain2.LoadProjects(root)
		if err != nil {
			return err
		}
		for _, project := range projects.Projects {
			if project.ID == options.ProjectID {
				options.ProjectName = project.Name
				options.ProjectGoal = project.Goal
				return nil
			}
		}
		return fmt.Errorf("Brain2 中没有项目 %q", options.ProjectID)
	default:
		return fmt.Errorf("不支持的工作入口 %q", options.WorkIntent)
	}
}

func preparePromptSkillSource(root, skillContent string) (string, error) {
	if strings.TrimSpace(skillContent) == "" {
		return "", errors.New("Launcher 未包含 Brain2 Skill")
	}
	path := filepath.Join(root, ".brain2", "runtime", "bootstrap", "brain2-connect", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	current, err := os.ReadFile(path)
	if err == nil && string(current) == skillContent {
		return path, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := atomicWrite(path, []byte(skillContent)); err != nil {
		return "", err
	}
	return path, nil
}

func manualInstallGuide(definition brain2.AgentDefinition, registryPath, launcherExe, skillSource string) string {
	launcherExe, _ = filepath.Abs(launcherExe)
	switch definition.ID {
	case "codex", "codex-desktop":
		return fmt.Sprintf("读取 `%s`，将该 Skill 安装到用户级 `~/.agents/skills/brain2-connect/SKILL.md`；然后执行 `codex mcp add %s -- \"%s\" mcp-serve --registry \"%s\"`。该连接器只安装一次，后续记忆库由 Launcher 任务路由。", skillSource, serverName, launcherExe, registryPath)
	case "claude-code":
		return fmt.Sprintf("读取 `%s`，将该 Skill 安装到用户级 `~/.claude/skills/brain2-connect/SKILL.md`；然后执行 `claude mcp add --scope user %s -- \"%s\" mcp-serve --registry \"%s\"`。该连接器只安装一次，后续记忆库由 Launcher 任务路由。", skillSource, serverName, launcherExe, registryPath)
	case "hermes":
		return fmt.Sprintf("读取 `%s` 并安装到 Hermes 用户级 `~/.hermes/skills/brain2-connect/SKILL.md`。Hermes 位于 WSL 时优先回到 Launcher 点击“快速接入”，由 Launcher 安装原生 Linux MCP Host；不要把 Windows Host 伪装成 Linux 二进制。连接器使用 Launcher 注册表 `%s`，只需安装一次。", skillSource, registryPath)
	case "claude-desktop":
		return "Claude Desktop 需要先在 Launcher 中生成并安装 Brain2 MCPB 扩展；扩展安装完成后重新复制本 Prompt。"
	default:
		return "读取 Skill 来源并按 Agent 的用户级 Skill/MCP 规范安装；不确定时返回 Launcher 使用快速接入。"
	}
}

func QuickConnect(ctx context.Context, root, registryPath, agentID, launcherExe, skillContent string, report ProgressReporter) (Result, error) {
	return QuickConnectForDevice(ctx, root, registryPath, "A", agentID, launcherExe, skillContent, report)
}

func Rebind(ctx context.Context, root, registryPath, agentID, launcherExe, skillContent string, report ProgressReporter) (Result, error) {
	return RebindForDevice(ctx, root, registryPath, "A", agentID, launcherExe, skillContent, report)
}

func QuickConnectForDevice(ctx context.Context, root, registryPath, deviceID, agentID, launcherExe, skillContent string, report ProgressReporter) (Result, error) {
	return quickConnect(ctx, root, registryPath, deviceID, agentID, launcherExe, skillContent, false, report)
}

func RebindForDevice(ctx context.Context, root, registryPath, deviceID, agentID, launcherExe, skillContent string, report ProgressReporter) (Result, error) {
	return quickConnect(ctx, root, registryPath, deviceID, agentID, launcherExe, skillContent, true, report)
}

func quickConnect(ctx context.Context, root, registryPath, deviceID, agentID, launcherExe, skillContent string, allowRegistryReplacement bool, report ProgressReporter) (Result, error) {
	emitProgress(report, agentID, "detecting", "正在确认 Agent 环境", "检测命令、版本与本地连接能力")
	detection, err := detectOne(ctx, agentID)
	if err != nil {
		emitProgress(report, agentID, "failed", "连接准备失败", err.Error())
		return Result{}, err
	}
	if !detection.Detected || detection.Executable == "" {
		err := fmt.Errorf("未检测到可用的 %s", detection.Name)
		emitProgress(report, agentID, "failed", "没有找到可用 Agent", err.Error())
		return Result{}, err
	}
	if !detection.QuickConnect {
		err := fmt.Errorf("%s 当前使用 Prompt / Desktop 扩展连接", detection.Name)
		emitProgress(report, agentID, "failed", "不支持后台快速连接", err.Error())
		return Result{}, err
	}
	skillPath, err := skillDestinationForDetection(ctx, detection)
	if err != nil && agentID != "claude-desktop" {
		return Result{}, err
	}
	if skillPath != "" {
		conflict, conflictErr := skillHasManagedConflict(agentID, skillPath, skillContent)
		if conflictErr != nil {
			return Result{}, conflictErr
		}
		if conflict {
			message := "检测到 brain2-connect Skill 已被手动修改；Launcher 未覆盖该文件。请先查看差异，或备份后恢复为 Launcher 管理版本"
			emitProgress(report, agentID, "failed", "Skill 更新冲突", message)
			return Result{AgentID: agentID, Status: "skill_conflict", Message: message, SkillConflict: true}, nil
		}
	}
	backupPath, err := beginConnectorBackup(ctx, detection, registryPath)
	if err != nil {
		return Result{}, fmt.Errorf("备份现有连接器失败，未执行更新: %w", err)
	}
	backupCompleted := false
	defer func() {
		if backupPath != "" && !backupCompleted {
			_ = finishConnectorBackup(backupPath, "failed", nil)
		}
	}()
	emitProgress(report, agentID, "installing_skill", "正在安装 Brain2 Skill", "校验受控版本并写入 Agent 用户级 Skill 目录；已有文件会先备份")
	skillResult, err := installSkillForDetection(ctx, detection, skillContent)
	if err != nil && agentID != "claude-desktop" {
		return Result{}, err
	}
	if skillResult.Conflict {
		message := "检测到 brain2-connect Skill 已被手动修改；Launcher 未覆盖该文件。请先查看差异，或备份后恢复为 Launcher 管理版本"
		return Result{AgentID: agentID, Status: "skill_conflict", Message: message, SkillConflict: true}, nil
	}
	workspaceSkillChanges, err := installWorkspaceSkillSource(root, skillContent)
	if err != nil {
		return Result{}, fmt.Errorf("更新 Brain2 内置 Skill 来源失败: %w", err)
	}
	emitProgress(report, agentID, "installing_host", "正在准备本地连接桥", "按版本并行安装稳定 MCP Host，不覆盖正在运行的进程")
	launcherExe, err = filepath.Abs(launcherExe)
	if err != nil {
		return Result{}, err
	}
	hostCommand := launcherExe
	var hostChanges []string
	if isWSLDetection(detection) {
		hostCommand, hostChanges, err = installWSLHost(ctx, detection)
	} else {
		hostCommand, hostChanges, err = installStableHost()
	}
	if err != nil {
		return Result{}, err
	}
	changedPaths := append([]string{}, workspaceSkillChanges...)
	changedPaths = append(changedPaths, skillResult.ChangedPaths...)
	changedPaths = append(hostChanges, changedPaths...)
	if agentID == "claude-desktop" {
		emitProgress(report, agentID, "preparing_task", "正在生成扩展激活任务", "创建 Claude Desktop 安装后的 Prompt 与回执约束")
		task, prompt, err := brain2.PrepareConnectionWithOptions(root, agentID, "quick", brain2.ConnectionPromptOptions{DeviceID: deviceID})
		if err != nil {
			return Result{}, err
		}
		emitProgress(report, agentID, "packaging", "正在生成 Claude Desktop 扩展", "打包本地 MCPB；稍后需要在 Claude Desktop 中确认安装")
		packagePath, packageErr := prepareClaudeDesktopBundle(registryPath, hostCommand)
		if packageErr != nil {
			_ = brain2.MarkConnectionFailedForDevice(root, deviceID, agentID, task.ConnectionID, packageErr.Error())
			return Result{}, packageErr
		}
		changedPaths = append(changedPaths, packagePath)
		command := exec.Command("explorer.exe", "/select,", packagePath)
		setHidden(command)
		_ = command.Start()
		return Result{
			AgentID: agentID, Status: string(brain2.ConnectionAwaitingHandshake), ConnectionID: task.ConnectionID,
			Message: "Claude Desktop MCPB 已生成；安装扩展后发送剪贴板中的激活 Prompt",
			Prompt:  prompt, VerificationPrompt: prompt, ChangedPaths: changedPaths, RestartRequired: true,
			SkillUpdated: skillResult.Updated, HostUpdated: len(hostChanges) > 0,
		}, nil
	}
	runner := runCommand
	emitProgress(report, agentID, "checking_local", "正在检查本地 MCP Host", "直接启动本地桥并确认五个 Brain2 工具可用；此步骤不调用模型")
	if err := smokeMCPHostForDetection(ctx, detection, hostCommand, registryPath); err != nil {
		_ = brain2.MarkConnectionFailedForDevice(root, deviceID, agentID, "", err.Error())
		emitProgress(report, agentID, "failed", "本地 MCP 检查失败", err.Error())
		return Result{}, err
	}
	emitProgress(report, agentID, "registering_mcp", "正在注册 Brain2 MCP", "连接 Launcher 本机记忆库注册表；不绑定单个记忆库目录")
	agentRegistryPath := registryPath
	if isWSLDetection(detection) {
		agentRegistryPath, err = windowsPathToWSL(registryPath)
		if err != nil {
			return Result{}, err
		}
	}
	beforeConfig, beforeConfigErr := readMCPRegistration(ctx, detection)
	if err := registerMCP(ctx, runner, detection, agentRegistryPath, hostCommand, allowRegistryReplacement); err != nil {
		_ = brain2.MarkConnectionFailedForDevice(root, deviceID, agentID, "", err.Error())
		emitProgress(report, agentID, "failed", "MCP 注册失败", err.Error())
		return Result{}, err
	}
	afterConfig, afterConfigErr := readMCPRegistration(ctx, detection)
	configUpdated := beforeConfigErr != nil || afterConfigErr != nil || !bytes.Equal(beforeConfig, afterConfig)
	if agentID == "codex-desktop" {
		if configPath, pathErr := codexConfigPath(); pathErr == nil {
			changedPaths = append(changedPaths, configPath)
		}
	}
	if backupPath != "" {
		if err := finishConnectorBackup(backupPath, "success", changedPaths); err != nil {
			return Result{}, fmt.Errorf("连接器已更新，但保存回退记录失败: %w", err)
		}
		backupCompleted = true
	}
	if err := brain2.PrepareRuntimeForDevice(root, deviceID, agentID); err != nil {
		return Result{}, err
	}
	connectorChanged := skillResult.Updated || len(hostChanges) > 0 || configUpdated
	if connectorChanged {
		err = brain2.MarkAgentRestartRequiredForDevice(root, deviceID, agentID, "quick")
	} else {
		err = brain2.MarkAgentRegisteredForDevice(root, deviceID, agentID, "quick")
	}
	if err != nil {
		return Result{}, err
	}
	task, verificationPrompt, err := brain2.PrepareConnectionWithOptions(root, agentID, "quick", brain2.ConnectionPromptOptions{
		DeviceID: deviceID, LocalReady: true, WorkIntent: "brain",
	})
	if err != nil {
		return Result{}, err
	}
	emitProgress(report, agentID, "registered", "全局连接器安装完成", detection.Name+" 已连接 Launcher 注册表；若当前会话早于本次安装，请完全退出并重开一次")
	message := detection.Name + " 的 Brain2 Skill 与连接器已检查；可以执行检查 Prompt 完成加载验证"
	if connectorChanged {
		message = detection.Name + " 的 Brain2 Skill 与连接器已安装；请完全退出并重开 Agent，再执行检查 Prompt 完成加载验证"
	}
	return Result{
		AgentID: agentID, Status: string(brain2.ConnectionRegistered), ConnectionID: task.ConnectionID,
		Message: message, Prompt: verificationPrompt, VerificationPrompt: verificationPrompt, ChangedPaths: changedPaths,
		RestartRequired: connectorChanged, SkillUpdated: skillResult.Updated, HostUpdated: len(hostChanges) > 0, ConfigUpdated: configUpdated,
	}, nil
}

func Rollback(ctx context.Context, root, registryPath, agentID string, report ProgressReporter) (Result, error) {
	detection, err := detectOne(ctx, agentID)
	if err != nil {
		return Result{}, err
	}
	backup, path, err := loadLastConnectorBackup(agentID)
	if err != nil {
		return Result{}, err
	}
	if backup.Status != "success" || (agentID != "codex-desktop" && strings.TrimSpace(backup.PreviousHost) == "") {
		return Result{}, errors.New("上次连接器更新没有可自动回退的已管理 Host")
	}
	if backup.PreviousHost != "" {
		if err := smokeMCPHostForDetection(ctx, detection, backup.PreviousHost, backup.RegistryPath); err != nil {
			return Result{}, fmt.Errorf("旧版 Host 自检失败，未执行回退: %w", err)
		}
	}
	if agentID == "codex-desktop" {
		emitProgress(report, agentID, "rolling_back", "正在回退连接器", "恢复上次更新前的 Codex Desktop 用户配置")
		if err := restoreCodexDesktopMCPRegistration([]byte(backup.PreviousConfig)); err != nil {
			return Result{}, err
		}
	} else {
		agentRegistry := backup.RegistryPath
		if isWSLDetection(detection) {
			agentRegistry, err = windowsPathToWSL(backup.RegistryPath)
			if err != nil {
				return Result{}, err
			}
		}
		emitProgress(report, agentID, "rolling_back", "正在回退连接器", "恢复上次更新前的 Host 与 Skill；完成后需要重启已打开的 Agent")
		if err := registerMCP(ctx, runCommand, detection, agentRegistry, backup.PreviousHost, true); err != nil {
			return Result{}, err
		}
	}
	for _, changed := range backup.ChangedPaths {
		marker := ".backup-"
		index := strings.LastIndex(changed, marker)
		if index < 0 {
			continue
		}
		destination := changed[:index]
		data, readErr := os.ReadFile(changed)
		if readErr != nil {
			return Result{}, fmt.Errorf("读取 Skill 备份失败: %w", readErr)
		}
		if writeErr := atomicWrite(destination, data); writeErr != nil {
			return Result{}, fmt.Errorf("恢复 Skill 备份失败: %w", writeErr)
		}
	}
	backup.Status = "rolled_back"
	backup.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeConnectorBackup(path, backup); err != nil {
		return Result{}, err
	}
	return Result{AgentID: agentID, Status: "rolled_back", Message: detection.Name + " 已回退到上次更新前的连接器；请完全退出并重开 Agent"}, nil
}

func beginConnectorBackup(ctx context.Context, detection Detection, registryPath string) (string, error) {
	config, err := readMCPRegistration(ctx, detection)
	if err != nil {
		if detection.ID == "codex-desktop" {
			return "", err
		}
		return "", nil
	}
	if len(config) == 0 && detection.ID != "codex-desktop" {
		return "", nil
	}
	backup := ConnectorBackup{
		SchemaVersion: "0.1", AgentID: detection.ID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "prepared",
		RegistryPath: registryPath, PreviousConfig: string(config),
	}
	if configUsesRegistry(config) {
		backup.PreviousHost = configuredHostCommand(config)
	}
	root, err := settings.RuntimeRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "connector-backups", detection.ID)
	path := filepath.Join(dir, time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
	if err := writeConnectorBackup(path, backup); err != nil {
		return "", err
	}
	return path, nil
}

func finishConnectorBackup(path, status string, changedPaths []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var backup ConnectorBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return err
	}
	backup.Status = status
	backup.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	backup.ChangedPaths = append([]string{}, changedPaths...)
	if err := writeConnectorBackup(path, backup); err != nil {
		return err
	}
	if status == "success" {
		return writeConnectorBackup(filepath.Join(filepath.Dir(path), "last-update.json"), backup)
	}
	return nil
}

func writeConnectorBackup(path string, backup ConnectorBackup) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}

func loadLastConnectorBackup(agentID string) (ConnectorBackup, string, error) {
	root, err := settings.RuntimeRoot()
	if err != nil {
		return ConnectorBackup{}, "", err
	}
	path := filepath.Join(root, "connector-backups", agentID, "last-update.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ConnectorBackup{}, path, errors.New("没有可回退的连接器更新记录")
	}
	var backup ConnectorBackup
	if json.Unmarshal(data, &backup) != nil {
		return ConnectorBackup{}, path, errors.New("连接器回退记录损坏")
	}
	return backup, path, nil
}

func connectorRollbackAvailable(agentID string) bool {
	backup, _, err := loadLastConnectorBackup(agentID)
	return err == nil && backup.Status == "success" && (agentID == "codex-desktop" || strings.TrimSpace(backup.PreviousHost) != "")
}

func readMCPRegistration(ctx context.Context, detection Detection) ([]byte, error) {
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	switch detection.ID {
	case "codex":
		return runCommand(readCtx, detection.Executable, withPrefix(detection, "mcp", "get", serverName, "--json"), "")
	case "codex-desktop":
		return readCodexDesktopMCPRegistration()
	case "claude-code":
		return runCommand(readCtx, detection.Executable, []string{"mcp", "get", serverName}, "")
	case "hermes":
		return readHermesConfig(readCtx, runCommand, detection)
	default:
		return nil, nil
	}
}

func configuredHostCommand(config []byte) string {
	value := strings.ReplaceAll(string(config), `\\`, `\`)
	lower := strings.ToLower(value)
	marker := "brain2-mcp-host-v"
	index := strings.Index(lower, marker)
	if index < 0 {
		return ""
	}
	start := index
	for start > 0 && value[start-1] != '"' && value[start-1] != '\'' && value[start-1] != '\n' && value[start-1] != '\r' {
		start--
	}
	end := index + len(marker)
	for end < len(value) && value[end] != '"' && value[end] != '\'' && value[end] != '\n' && value[end] != '\r' && value[end] != ',' && value[end] != ']' && value[end] != '}' {
		end++
	}
	host := strings.TrimSpace(value[start:end])
	if colon := strings.LastIndex(host, ": "); colon >= 0 {
		host = strings.TrimSpace(host[colon+2:])
	}
	return host
}

func VerifyAgent(ctx context.Context, root, agentID string, report ProgressReporter) (Result, error) {
	return VerifyAgentForDevice(ctx, root, "A", agentID, report)
}

func VerifyAgentForDevice(ctx context.Context, root, deviceID, agentID string, report ProgressReporter) (Result, error) {
	emitProgress(report, agentID, "preparing_task", "正在生成在线验证任务", "创建一次性 nonce；本阶段会调用 Agent 服务")
	detection, err := detectOne(ctx, agentID)
	if err != nil {
		return Result{}, err
	}
	if !detection.Detected || detection.Executable == "" {
		return Result{}, fmt.Errorf("未检测到 %s 的可用 CLI", detection.Name)
	}
	registered, err := isLocallyRegisteredForDevice(root, deviceID, agentID)
	if err != nil {
		return Result{}, err
	}
	if !registered {
		return Result{}, errors.New("请先完成本地快速接入，再验证 Agent")
	}
	task, prompt, err := brain2.PrepareConnectionWithOptions(root, agentID, "quick", brain2.ConnectionPromptOptions{DeviceID: deviceID})
	if err != nil {
		return Result{}, err
	}
	runner := runCommand
	emitProgress(report, agentID, "handshaking", "Agent 正在读取 Brain2", "等待在线 Agent 直接调用 MCP、完成健康检查并提交回执；本地接入状态不会因验证失败而丢失")
	output, handshakeErr := runHandshake(ctx, runner, detection, root, prompt)
	logPath := filepath.Join(root, ".brain2", "connections", task.ConnectionID, "agent-output.log")
	logData := output
	if handshakeErr != nil {
		logData = append(logData, []byte("\n[launcher-error] "+handshakeErr.Error()+"\n")...)
	}
	_ = atomicWrite(logPath, logData)
	if handshakeErr != nil {
		diagnostic := diagnoseHandshakeFailure(detection.Name, output, handshakeErr)
		_ = brain2.MarkVerificationFailedForDevice(root, deviceID, agentID, task.ConnectionID, diagnostic.Error())
		emitProgress(report, agentID, "failed", "Agent 握手失败", diagnostic.Error())
		return Result{}, diagnostic
	}
	emitProgress(report, agentID, "verifying_receipt", "正在验证连接回执", "校验 Agent、nonce、协议版本、健康状态与读取证据")
	receiptPath := filepath.Join(root, ".brain2", "connections", task.ConnectionID, "receipt.json")
	if _, err := os.Stat(receiptPath); err != nil {
		message := diagnoseMissingReceipt(detection.Name, output)
		_ = brain2.MarkVerificationFailedForDevice(root, deviceID, agentID, task.ConnectionID, message)
		emitProgress(report, agentID, "failed", "没有收到有效回执", message)
		return Result{}, errors.New(message)
	}
	emitProgress(report, agentID, "connected", "Brain2 连接完成", detection.Name+" 已提交并通过结构化回执校验")
	return Result{
		AgentID: agentID, Status: string(brain2.ConnectionConnected), ConnectionID: task.ConnectionID,
		Message: detection.Name + " 已完成 Brain2 在线验证", ReceiptPath: receiptPath,
	}, nil
}

func isLocallyRegistered(root, agentID string) (bool, error) {
	return isLocallyRegisteredForDevice(root, "A", agentID)
}

func isLocallyRegisteredForDevice(root, deviceID, agentID string) (bool, error) {
	registry, err := brain2.LoadAgentsForDevice(root, deviceID)
	if err != nil {
		return false, err
	}
	for _, agent := range registry.Agents {
		if agent.ID == agentID {
			return agent.ConnectionStatus == string(brain2.ConnectionRegistered) || agent.ConnectionStatus == string(brain2.ConnectionConnected), nil
		}
	}
	return false, nil
}

func detectOne(ctx context.Context, agentID string) (Detection, error) {
	for _, detection := range DetectAll(ctx) {
		if detection.ID == agentID {
			return detection, nil
		}
	}
	return Detection{}, fmt.Errorf("不支持的 Agent %q", agentID)
}

func detectionFor(ctx context.Context, id, name, surface, executable string, prefix []string, quick, prompt bool) Detection {
	item := Detection{ID: id, Name: name, Surface: surface, Detected: executable != "", QuickConnect: quick, PromptConnect: prompt, Executable: executable, PrefixArgs: prefix}
	if executable == "" {
		item.Status = "not_detected"
		item.Message = "未检测到可用 CLI"
		return item
	}
	versionCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	output, err := runCommand(versionCtx, executable, withPrefix(item, "--version"), "")
	if err != nil {
		item.Status = "unavailable"
		item.QuickConnect = false
		item.Message = "检测到安装痕迹，但命令无法启动"
		return item
	}
	item.Version = firstLine(string(output))
	item.Status = "installable"
	item.Message = "可快速连接"
	return item
}

func desktopDetection(id, name, installationEvidence string) Detection {
	item := Detection{ID: id, Name: name, Surface: "desktop", Detected: installationEvidence != "", QuickConnect: installationEvidence != "", PromptConnect: true, Executable: installationEvidence}
	if !item.Detected {
		item.Status = "not_detected"
		item.Message = "未检测到桌面应用"
		return item
	}
	item.Status = "installable"
	item.Message = "已检测到桌面应用"
	return item
}

func registerMCP(ctx context.Context, runner commandRunner, detection Detection, registryPath, launcherExe string, allowRegistryReplacement bool) error {
	registerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	serverArgs := []string{"mcp-serve", "--registry", registryPath}
	switch detection.ID {
	case "codex":
		if output, err := runner(registerCtx, detection.Executable, withPrefix(detection, "mcp", "get", serverName, "--json"), ""); err == nil {
			if configUsesRegistry(output) && !configContainsPath(output, registryPath) && !allowRegistryReplacement {
				return errors.New("Codex 已有同名 Brain2 MCP 配置，但使用另一套 Launcher 注册表；未自动覆盖")
			}
			if configContainsPath(output, registryPath) && configContainsExecutable(output, launcherExe) {
				return nil
			}
			if _, removeErr := runner(registerCtx, detection.Executable, withPrefix(detection, "mcp", "remove", serverName), ""); removeErr != nil {
				return commandError("更新 Codex MCP 前移除旧版 Host", removeErr)
			}
		}
		args := append([]string{"mcp", "add", serverName, "--", launcherExe}, serverArgs...)
		_, err := runner(registerCtx, detection.Executable, withPrefix(detection, args...), "")
		return commandError("注册 Codex MCP", err)
	case "codex-desktop":
		if err := registerCodexDesktopMCP(registryPath, launcherExe, allowRegistryReplacement); err != nil {
			return err
		}
		return nil
	case "claude-code":
		if output, err := runner(registerCtx, detection.Executable, []string{"mcp", "get", serverName}, ""); err == nil {
			if configUsesRegistry(output) && !configContainsPath(output, registryPath) && !allowRegistryReplacement {
				return errors.New("Claude Code 已有同名 Brain2 MCP 配置，但使用另一套 Launcher 注册表；未自动覆盖")
			}
			if configContainsPath(output, registryPath) && configContainsExecutable(output, launcherExe) {
				return nil
			}
			if _, removeErr := runner(registerCtx, detection.Executable, []string{"mcp", "remove", "--scope", "user", serverName}, ""); removeErr != nil {
				return commandError("更新 Claude Code MCP 前移除旧版 Host", removeErr)
			}
		}
		_, err := runner(registerCtx, detection.Executable, append([]string{"mcp", "add", "--scope", "user", serverName, "--", launcherExe}, serverArgs...), "")
		return commandError("注册 Claude Code MCP", err)
	case "hermes":
		if output, err := runner(registerCtx, detection.Executable, withPrefix(detection, "mcp", "list"), ""); err == nil && strings.Contains(strings.ToLower(string(output)), serverName) {
			if hermesMCPEnabled(output) {
				config, configErr := readHermesConfig(registerCtx, runner, detection)
				if configErr != nil {
					return commandError("读取 Hermes MCP 配置", configErr)
				}
				if configUsesRegistry(config) && !configContainsPath(config, registryPath) && !allowRegistryReplacement {
					return errors.New("Hermes 已有同名 Brain2 MCP 配置，但使用另一套 Launcher 注册表；未自动覆盖")
				}
				if configContainsPath(config, registryPath) && configContainsExecutable(config, launcherExe) {
					return nil
				}
				if _, removeErr := runner(registerCtx, detection.Executable, withPrefix(detection, "mcp", "remove", serverName), "y\n"); removeErr != nil {
					return commandError("更新 Hermes MCP 前移除旧版 Host", removeErr)
				}
			} else if _, removeErr := runner(registerCtx, detection.Executable, withPrefix(detection, "mcp", "remove", serverName), "y\n"); removeErr != nil {
				return commandError("移除未启用的 Hermes MCP 配置", removeErr)
			}
		}
		args := []string{"mcp", "add", serverName, "--command", launcherExe, "--args", "mcp-serve", "--registry", registryPath}
		addOutput, err := runner(registerCtx, detection.Executable, withPrefix(detection, args...), "y\n")
		if err != nil {
			return commandError("注册 Hermes MCP", err)
		}
		verifyOutput, verifyErr := runner(registerCtx, detection.Executable, withPrefix(detection, "mcp", "list"), "")
		if verifyErr != nil {
			return commandError("复核 Hermes MCP 注册", verifyErr)
		}
		if !hermesMCPEnabled(verifyOutput) {
			message := strings.TrimSpace(string(addOutput))
			if message == "" {
				message = "Hermes mcp list 未显示 brain2-meow 为 enabled"
			}
			return fmt.Errorf("Hermes MCP 注册未生效: %s", truncate(message, 800))
		}
		return nil
	default:
		return fmt.Errorf("%s 没有 CLI 快速连接适配器", detection.Name)
	}
}

func hermesMCPEnabled(output []byte) bool {
	value := strings.ToLower(string(output))
	return strings.Contains(value, serverName) && strings.Contains(value, "enabled")
}

func smokeMCPHost(ctx context.Context, hostExe, registryPath string) error {
	smokeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"meow-launcher-local-check","version":"0.6.6"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		"",
	}, "\n")
	output, err := runCommand(smokeCtx, hostExe, []string{"mcp-serve", "--registry", registryPath}, input)
	if err != nil {
		return commandError("启动本地 Brain2 MCP Host", err)
	}
	value := string(output)
	for _, tool := range []string{"brain2_get_bootstrap", "brain2_read_file", "brain2_health", "brain2_complete_handshake", "brain2_submit_project_candidates"} {
		if !strings.Contains(value, `"name":"`+tool+`"`) {
			return fmt.Errorf("本地 Brain2 MCP Host 缺少工具 %s", tool)
		}
	}
	return nil
}

func smokeMCPHostForDetection(ctx context.Context, detection Detection, hostCommand, registryPath string) error {
	if !isWSLDetection(detection) {
		return smokeMCPHost(ctx, hostCommand, registryPath)
	}
	agentRegistry, err := windowsPathToWSL(registryPath)
	if err != nil {
		return err
	}
	smokeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"meow-launcher-local-check","version":"0.6.6"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		"",
	}, "\n")
	args := append(wslBaseArgs(detection), hostCommand, "--registry", agentRegistry)
	output, err := runCommand(smokeCtx, detection.Executable, args, input)
	if err != nil {
		return commandError("启动 WSL Brain2 MCP Host", err)
	}
	value := string(output)
	for _, tool := range []string{"brain2_get_bootstrap", "brain2_read_file", "brain2_health", "brain2_complete_handshake", "brain2_submit_project_candidates"} {
		if !strings.Contains(value, `"name":"`+tool+`"`) {
			return fmt.Errorf("WSL Brain2 MCP Host 缺少工具 %s", tool)
		}
	}
	return nil
}

func runHandshake(ctx context.Context, runner commandRunner, detection Detection, root, prompt string) ([]byte, error) {
	handshakeCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var args []string
	stdin := ""
	switch detection.ID {
	case "codex", "codex-desktop":
		args = withPrefix(detection, "exec", "-", "--json", "--ephemeral", "--skip-git-repo-check", "-C", root, "--sandbox", "read-only")
		stdin = prompt
	case "claude-code":
		args = []string{"-p", prompt, "--output-format", "json", "--max-turns", "8", "--add-dir", root}
	case "hermes":
		args = withPrefix(detection, "chat", "-q", prompt)
	default:
		return nil, fmt.Errorf("%s 没有后台握手适配器", detection.Name)
	}
	output, err := runner(handshakeCtx, detection.Executable, args, stdin)
	return output, commandError("Agent 握手", err)
}

func emitProgress(report ProgressReporter, agentID, phase, title, detail string) {
	if report != nil {
		report(Progress{AgentID: agentID, Phase: phase, Title: title, Detail: detail})
	}
}

func diagnoseHandshakeFailure(agentName string, output []byte, runErr error) error {
	combined := strings.ToLower(string(output) + "\n" + runErr.Error())
	switch {
	case strings.Contains(combined, "request timed out"), strings.Contains(combined, "stream disconnected"), strings.Contains(combined, "reconnecting"), strings.Contains(combined, "context deadline exceeded"):
		return fmt.Errorf("%s 服务连接超时；Brain2 MCP 已注册，但 Agent 尚未完成握手。请检查网络与 Agent 登录状态后重试", agentName)
	case strings.Contains(combined, "not logged in"), strings.Contains(combined, "login required"), strings.Contains(combined, "authentication"):
		return fmt.Errorf("%s 尚未登录或登录已失效；请先在官方 Agent 中完成登录，再返回 Launcher 重试", agentName)
	case strings.Contains(combined, "mcp") && (strings.Contains(combined, "failed") || strings.Contains(combined, "error")):
		return fmt.Errorf("%s 未能启动 Brain2 MCP；详细输出已保存到本次 connection 的 agent-output.log", agentName)
	default:
		return fmt.Errorf("%s 后台握手未完成：%s", agentName, truncate(runErr.Error(), 600))
	}
}

func diagnoseMissingReceipt(agentName string, output []byte) string {
	value := strings.ToLower(string(output))
	switch {
	case strings.Contains(value, "user cancelled mcp tool call"):
		return agentName + " 的 MCP 调用在提交回执前被取消；本地接入仍然有效，可稍后重新验证 Agent"
	case strings.Contains(value, "request timed out"), strings.Contains(value, "stream disconnected"), strings.Contains(value, "reconnecting"):
		return agentName + " 服务连接超时；MCP 已注册，但 Agent 没有运行到回执步骤。请检查网络与登录状态后重试"
	case strings.Contains(value, "not logged in"), strings.Contains(value, "login required"):
		return agentName + " 尚未登录或登录已失效；请先完成官方登录后重试"
	case strings.TrimSpace(value) == "":
		return agentName + " 命令没有返回任何输出，也没有生成连接回执；请改用“复制 Prompt”或检查 Agent 安装状态"
	default:
		return agentName + " 命令已结束但没有提交有效回执；完整 Agent 输出已保存在本次 connection 的 agent-output.log"
	}
}

func installSkill(agentID, content string) ([]string, error) {
	result, err := installSkillManaged(agentID, "", content)
	return result.ChangedPaths, err
}

func installSkillManaged(agentID, destination, content string) (skillInstallResult, error) {
	if strings.TrimSpace(content) == "" {
		return skillInstallResult{}, errors.New("Launcher 未包含 Brain2 Skill")
	}
	if destination == "" {
		var err error
		destination, err = skillDestination(agentID)
		if err != nil {
			return skillInstallResult{}, err
		}
	}
	if destination == "" {
		return skillInstallResult{}, nil
	}
	conflict, err := skillHasManagedConflict(agentID, destination, content)
	if err != nil {
		return skillInstallResult{}, err
	}
	if conflict {
		return skillInstallResult{Conflict: true, Path: destination}, nil
	}
	current, readErr := os.ReadFile(destination)
	if readErr != nil && !os.IsNotExist(readErr) {
		return skillInstallResult{}, readErr
	}
	newHash := sha256Hex([]byte(content))
	if readErr == nil && sha256Hex(current) == newHash {
		if err := writeSkillManifest(agentID, destination, newHash); err != nil {
			return skillInstallResult{}, err
		}
		return skillInstallResult{Path: destination}, nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return skillInstallResult{}, err
	}
	changed := []string{}
	if readErr == nil {
		backup := destination + ".backup-" + time.Now().Format("20060102-150405")
		if err := atomicWrite(backup, current); err != nil {
			return skillInstallResult{}, err
		}
		changed = append(changed, backup)
	}
	if err := atomicWrite(destination, []byte(content)); err != nil {
		return skillInstallResult{}, err
	}
	changed = append(changed, destination)
	if err := writeSkillManifest(agentID, destination, newHash); err != nil {
		return skillInstallResult{}, err
	}
	return skillInstallResult{ChangedPaths: changed, Updated: true, Path: destination}, nil
}

func skillDestination(agentID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch agentID {
	case "codex", "codex-desktop":
		return filepath.Join(home, ".agents", "skills", "brain2-connect", "SKILL.md"), nil
	case "claude-code":
		return filepath.Join(home, ".claude", "skills", "brain2-connect", "SKILL.md"), nil
	case "hermes":
		return filepath.Join(home, ".hermes", "skills", "brain2-connect", "SKILL.md"), nil
	case "claude-desktop":
		return "", nil
	default:
		return "", fmt.Errorf("%s 不支持 CLI Skill 安装", agentID)
	}
}

func skillDestinationForDetection(ctx context.Context, detection Detection) (string, error) {
	if detection.ID != "hermes" || !isWSLDetection(detection) {
		return skillDestination(detection.ID)
	}
	distro := wslDistribution(detection)
	homeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := runCommand(homeCtx, detection.Executable, append(wslBaseArgs(detection), "sh", "-lc", `printf %s "$HOME"`), "")
	if err != nil {
		return "", commandError("读取 Hermes WSL 用户目录", err)
	}
	home := strings.TrimSpace(decodeWSLOutput(output))
	if !strings.HasPrefix(home, "/") {
		return "", errors.New("Hermes WSL 用户目录无效")
	}
	return wslUNCPath(distro, home+"/.hermes/skills/brain2-connect/SKILL.md"), nil
}

func installSkillForDetection(ctx context.Context, detection Detection, content string) (skillInstallResult, error) {
	destination, err := skillDestinationForDetection(ctx, detection)
	if err != nil {
		return skillInstallResult{}, err
	}
	return installSkillManaged(detection.ID, destination, content)
}

func skillHasManagedConflict(agentID, destination, content string) (bool, error) {
	current, err := os.ReadFile(destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	currentHash := sha256Hex(current)
	if currentHash == sha256Hex([]byte(content)) {
		return false, nil
	}
	manifest, err := loadSkillManifest(agentID)
	if os.IsNotExist(err) {
		_, knownLegacy := legacyManagedSkillHashes[currentHash]
		return !knownLegacy, nil
	}
	if err != nil {
		return false, err
	}
	if !strings.EqualFold(filepath.Clean(manifest.Path), filepath.Clean(destination)) {
		return true, nil
	}
	return !strings.EqualFold(currentHash, manifest.ManagedSHA256), nil
}

func skillManifestPath(agentID string) (string, error) {
	root, err := settings.RuntimeRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "connector-backups", agentID, "skill-manifest.json"), nil
}

func loadSkillManifest(agentID string) (skillManifest, error) {
	path, err := skillManifestPath(agentID)
	if err != nil {
		return skillManifest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return skillManifest{}, err
	}
	var manifest skillManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return skillManifest{}, errors.New("Brain2 Skill 受控版本记录损坏；未覆盖现有 Skill")
	}
	return manifest, nil
}

func writeSkillManifest(agentID, destination, managedHash string) error {
	path, err := skillManifestPath(agentID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	manifest := skillManifest{
		SchemaVersion: "0.1", AgentID: agentID, Path: destination,
		ManagedSHA256: managedHash, InstalledVersion: "brain2-connect@" + managedHash[:12],
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func installWorkspaceSkillSource(root, content string) ([]string, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("Launcher 未包含 Brain2 Skill")
	}
	destination := filepath.Join(root, ".brain2", "runtime", "bootstrap", "brain2-connect", "SKILL.md")
	current, err := os.ReadFile(destination)
	if err == nil && sha256Hex(current) == sha256Hex([]byte(content)) {
		return nil, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return nil, err
	}
	changed := []string{}
	if err == nil {
		backup := destination + ".backup-" + time.Now().Format("20060102-150405")
		if err := atomicWrite(backup, current); err != nil {
			return nil, err
		}
		changed = append(changed, backup)
	}
	if err := atomicWrite(destination, []byte(content)); err != nil {
		return nil, err
	}
	return append(changed, destination), nil
}

func prepareClaudeDesktopBundle(registryPath, hostExe string) (string, error) {
	outputDir := filepath.Join(filepath.Dir(filepath.Dir(hostExe)), "packages")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	packagePath := filepath.Join(outputDir, "brain2-claude-desktop.mcpb")
	temp, err := os.CreateTemp(outputDir, ".brain2-mcpb-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	archive := zip.NewWriter(temp)
	manifest := map[string]any{
		"$schema":          "https://raw.githubusercontent.com/anthropics/mcpb/main/schemas/mcpb-manifest-v0.4.schema.json",
		"manifest_version": "0.4",
		"name":             "brain2-meow",
		"display_name":     "Brain2 by Meow Launcher",
		"version":          MCPHostVersion,
		"description":      "Connect Claude Desktop to a governed local Brain2 workspace.",
		"author":           map[string]any{"name": "Meow Launcher"},
		"server": map[string]any{
			"type": "binary", "entry_point": "server/brain2-mcp-host.exe",
			"mcp_config": map[string]any{
				"command": "${__dirname}/server/brain2-mcp-host",
				"args":    []string{"mcp-serve", "--registry", "${user_config.brain2_registry}"},
				"env":     map[string]string{},
			},
		},
		"compatibility": map[string]any{"platforms": []string{"win32"}},
		"user_config": map[string]any{
			"brain2_registry": map[string]any{
				"type": "string", "title": "Brain2 Registry", "description": "Meow Launcher local workspace registry path.",
				"required": true, "default": registryPath,
			},
		},
		"tools": []map[string]string{
			{"name": "brain2_get_bootstrap", "description": "Read the governed Brain2 bootstrap."},
			{"name": "brain2_read_file", "description": "Read an allowed file inside Brain2."},
			{"name": "brain2_health", "description": "Run deterministic Brain2 health checks."},
			{"name": "brain2_submit_project_candidates", "description": "Submit project candidates for Launcher review."},
			{"name": "brain2_complete_handshake", "description": "Write a validated connection receipt."},
		},
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		archive.Close()
		temp.Close()
		return "", err
	}
	manifestFile, err := archive.Create("manifest.json")
	if err != nil {
		archive.Close()
		temp.Close()
		return "", err
	}
	if _, err := manifestFile.Write(manifestData); err != nil {
		archive.Close()
		temp.Close()
		return "", err
	}
	hostData, err := os.ReadFile(hostExe)
	if err != nil {
		archive.Close()
		temp.Close()
		return "", err
	}
	hostFile, err := archive.Create("server/brain2-mcp-host.exe")
	if err != nil {
		archive.Close()
		temp.Close()
		return "", err
	}
	if _, err := hostFile.Write(hostData); err != nil {
		archive.Close()
		temp.Close()
		return "", err
	}
	if err := archive.Close(); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(packagePath); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(tempPath, packagePath); err != nil {
		return "", err
	}
	return packagePath, nil
}

func installStableHost() (string, []string, error) {
	dataRoot, err := settings.RuntimeRoot()
	if err != nil {
		return "", nil, err
	}
	data, err := runtimeAssets.ReadFile("assets/brain2-mcp-host-windows-amd64.exe")
	if err != nil {
		return "", nil, err
	}
	return installStableHostDataTo(data, dataRoot)
}

func installStableHostTo(source, dataRoot string) (string, []string, error) {
	sourceData, err := os.ReadFile(source)
	if err != nil {
		return "", nil, err
	}
	return installStableHostDataTo(sourceData, dataRoot)
}

func installStableHostDataTo(sourceData []byte, dataRoot string) (string, []string, error) {
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return "", nil, err
	}
	destination := filepath.Join(dataRoot, versionedHostName())
	changed := []string{}
	if _, readErr := os.ReadFile(destination); readErr == nil {
		// Keep the installed semantic version stable across UI-only Launcher
		// rebuilds. MCP behavior changes must bump MCPHostVersion.
		return destination, changed, nil
	} else if !os.IsNotExist(readErr) {
		return "", nil, readErr
	}
	if err := atomicWrite(destination, sourceData); err != nil {
		return "", nil, err
	}
	_ = os.Chmod(destination, 0o755)
	changed = append(changed, destination)
	return destination, changed, nil
}

func installWSLHost(ctx context.Context, detection Detection) (string, []string, error) {
	data, err := runtimeAssets.ReadFile("assets/brain2-mcp-host-linux-amd64")
	if err != nil {
		return "", nil, err
	}
	distro := wslDistribution(detection)
	homeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := runCommand(homeCtx, detection.Executable, append(wslBaseArgs(detection), "sh", "-lc", `printf %s "$HOME"`), "")
	if err != nil {
		return "", nil, commandError("读取 Hermes WSL 用户目录", err)
	}
	home := strings.TrimSpace(decodeWSLOutput(output))
	if !strings.HasPrefix(home, "/") {
		return "", nil, errors.New("Hermes WSL 用户目录无效")
	}
	linuxPath := fmt.Sprintf("%s/.hermes/bin/%s", strings.TrimRight(home, "/"), versionedLinuxHostName())
	windowsPath := wslUNCPath(distro, linuxPath)
	changed := []string{}
	if _, readErr := os.ReadFile(windowsPath); readErr == nil {
		// The semantic Host version is the compatibility contract. UI-only
		// Launcher rebuilds must not replace or invalidate an installed Host.
	} else if os.IsNotExist(readErr) {
		if err := os.MkdirAll(filepath.Dir(windowsPath), 0o755); err != nil {
			return "", nil, err
		}
		if err := atomicWrite(windowsPath, data); err != nil {
			return "", nil, err
		}
		changed = append(changed, windowsPath)
	} else {
		return "", nil, readErr
	}
	chmodCtx, chmodCancel := context.WithTimeout(ctx, 15*time.Second)
	defer chmodCancel()
	if _, err := runCommand(chmodCtx, detection.Executable, append(wslBaseArgs(detection), "chmod", "755", linuxPath), ""); err != nil {
		return "", nil, commandError("设置 WSL MCP Host 执行权限", err)
	}
	return linuxPath, changed, nil
}

func versionedHostName() string {
	return fmt.Sprintf("brain2-mcp-host-v%s.exe", MCPHostVersion)
}

func versionedLinuxHostName() string {
	return fmt.Sprintf("brain2-mcp-host-v%s", MCPHostVersion)
}

func runCommand(ctx context.Context, executable string, args []string, stdin string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	setHidden(command)
	err := command.Run()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message != "" {
			return stdout.Bytes(), fmt.Errorf("%w: %s", err, truncate(message, 1200))
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}

func findExecutable(command string, known []string) string {
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath(command + ".exe"); err == nil {
			return path
		}
	}
	for _, path := range known {
		if path != "" {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path
			}
		}
	}
	if path, err := exec.LookPath(command); err == nil && filepath.Ext(path) != ".cmd" && filepath.Ext(path) != ".ps1" {
		return path
	}
	return ""
}

func findHermesCommand(ctx context.Context) (string, []string, string) {
	windowsPath := findExecutable("hermes", []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "hermes", "bin", "hermes.exe"),
		filepath.Join(os.Getenv("USERPROFILE"), ".local", "bin", "hermes.exe"),
	})
	if windowsPath != "" {
		return windowsPath, nil, ""
	}
	wslPath := findExecutable("wsl", []string{
		filepath.Join(os.Getenv("WINDIR"), "System32", "wsl.exe"),
		`C:\Program Files\WSL\wsl.exe`,
	})
	if wslPath == "" {
		return "", nil, ""
	}
	return findWSLHermesCommand(ctx, wslPath, runCommand)
}

func findWSLHermesCommand(ctx context.Context, wslPath string, runner commandRunner) (string, []string, string) {
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := runner(listCtx, wslPath, []string{"--list", "--quiet"}, "")
	if err != nil {
		return "", nil, ""
	}
	distros := parseWSLDistributions(output)
	ordered := make([]string, 0, len(distros))
	for _, distro := range distros {
		if strings.Contains(strings.ToLower(distro), "hermes") {
			ordered = append(ordered, distro)
		}
	}
	for _, distro := range distros {
		if !strings.Contains(strings.ToLower(distro), "hermes") {
			ordered = append(ordered, distro)
		}
	}
	for _, distro := range ordered {
		probeCtx, probeCancel := context.WithTimeout(ctx, 15*time.Second)
		probe := `if [ -x /usr/local/bin/hermes ]; then printf %s /usr/local/bin/hermes; else command -v hermes; fi`
		pathOutput, probeErr := runner(probeCtx, wslPath, []string{"-d", distro, "--", "sh", "-lc", probe}, "")
		probeCancel()
		if probeErr != nil {
			continue
		}
		linuxPath := firstLine(decodeWSLOutput(pathOutput))
		if strings.HasPrefix(linuxPath, "/") {
			return wslPath, []string{"-d", distro, "--", linuxPath}, distro
		}
	}
	return "", nil, ""
}

func parseWSLDistributions(output []byte) []string {
	lines := strings.FieldsFunc(decodeWSLOutput(output), func(r rune) bool { return r == '\r' || r == '\n' })
	result := make([]string, 0, len(lines))
	seen := map[string]bool{}
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if line != "" && !seen[strings.ToLower(line)] {
			seen[strings.ToLower(line)] = true
			result = append(result, line)
		}
	}
	return result
}

func decodeWSLOutput(output []byte) string {
	if !bytes.Contains(output, []byte{0}) {
		return strings.TrimPrefix(string(output), "\ufeff")
	}
	if len(output)%2 != 0 {
		output = output[:len(output)-1]
	}
	values := make([]uint16, 0, len(output)/2)
	for i := 0; i+1 < len(output); i += 2 {
		values = append(values, binary.LittleEndian.Uint16(output[i:i+2]))
	}
	return strings.TrimPrefix(string(utf16.Decode(values)), "\ufeff")
}

func isWSLDetection(detection Detection) bool {
	return strings.EqualFold(filepath.Base(detection.Executable), "wsl.exe") && wslDistribution(detection) != ""
}

func wslDistribution(detection Detection) string {
	for i := 0; i+1 < len(detection.PrefixArgs); i++ {
		if detection.PrefixArgs[i] == "-d" || detection.PrefixArgs[i] == "--distribution" {
			return detection.PrefixArgs[i+1]
		}
	}
	return ""
}

func wslBaseArgs(detection Detection) []string {
	distro := wslDistribution(detection)
	if distro == "" {
		return nil
	}
	return []string{"-d", distro, "--"}
}

func wslUNCPath(distro, linuxPath string) string {
	linuxPath = strings.TrimPrefix(filepath.ToSlash(linuxPath), "/")
	return `\\wsl.localhost\` + distro + `\` + strings.ReplaceAll(linuxPath, "/", `\`)
}

func windowsPathToWSL(path string) (string, error) {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if len(volume) != 2 || volume[1] != ':' {
		return "", fmt.Errorf("无法把 Windows 路径转换为 WSL 路径: %s", path)
	}
	rest := strings.TrimPrefix(clean, volume)
	rest = strings.TrimLeft(strings.ReplaceAll(rest, `\`, "/"), "/")
	return "/mnt/" + strings.ToLower(volume[:1]) + "/" + rest, nil
}

func findCodexCommand() (string, []string) {
	toolsRoot := strings.TrimSpace(os.Getenv("MEOW_AGENT_TOOLS_ROOT"))
	nodeCandidates := []string{}
	if toolsRoot != "" {
		nodeCandidates = append(nodeCandidates, filepath.Join(toolsRoot, "node", "node.exe"))
	}
	node := findExecutable("node", nodeCandidates)
	scripts := []string{
		filepath.Join(os.Getenv("APPDATA"), "npm", "node_modules", "@openai", "codex", "bin", "codex.js"),
	}
	if wrapperDir := commandWrapperDir("codex"); wrapperDir != "" {
		scripts = append([]string{filepath.Join(wrapperDir, "node_modules", "@openai", "codex", "bin", "codex.js")}, scripts...)
	}
	if toolsRoot != "" {
		scripts = append(scripts, filepath.Join(toolsRoot, "codex-cli", "node_modules", "@openai", "codex", "bin", "codex.js"))
	}
	if node != "" {
		for _, script := range scripts {
			if info, err := os.Stat(script); err == nil && info.Mode().IsRegular() {
				return node, []string{script}
			}
		}
	}
	candidates := []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Codex CLI", "codex.exe")}
	if toolsRoot != "" {
		candidates = append(candidates, filepath.Join(toolsRoot, "codex-cli", "codex.exe"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", nil
}

func findClaudeCommand() string {
	toolsRoot := strings.TrimSpace(os.Getenv("MEOW_AGENT_TOOLS_ROOT"))
	candidates := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Claude Code", "claude.exe"),
		filepath.Join(os.Getenv("USERPROFILE"), ".local", "bin", "claude.exe"),
		filepath.Join(os.Getenv("APPDATA"), "npm", "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe"),
	}
	if wrapperDir := commandWrapperDir("claude"); wrapperDir != "" {
		candidates = append([]string{filepath.Join(wrapperDir, "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")}, candidates...)
	}
	if toolsRoot != "" {
		candidates = append(candidates, filepath.Join(toolsRoot, "claude-code", "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe"))
	}
	return findExecutable("claude", candidates)
}

func commandWrapperDir(command string) string {
	path, err := exec.LookPath(command)
	if err != nil {
		return ""
	}
	return filepath.Dir(path)
}

func withPrefix(detection Detection, args ...string) []string {
	result := make([]string, 0, len(detection.PrefixArgs)+len(args))
	result = append(result, detection.PrefixArgs...)
	result = append(result, args...)
	return result
}

func configContainsRoot(output []byte, root string) bool {
	return configContainsPath(output, root)
}

func configUsesRegistry(output []byte) bool {
	value := strings.ToLower(string(output))
	return strings.Contains(value, "--registry") || strings.Contains(value, "brain2_registry")
}

func configContainsExecutable(output []byte, executable string) bool {
	return configContainsPath(output, executable)

}

func configContainsPath(output []byte, path string) bool {
	value := strings.ToLower(string(output))
	value = strings.ReplaceAll(value, `\\`, `\`)
	value = strings.ReplaceAll(value, `\`, "/")
	target := strings.ToLower(strings.ReplaceAll(filepath.Clean(path), `\`, "/"))
	return strings.Contains(value, target)
}

func findExisting(paths []string) string {
	for _, path := range paths {
		if path != "" {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path
			}
		}
	}
	return ""
}

func atomicWrite(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".brain2-skill-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tempPath, path)
}

func commandError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s失败: %w", action, err)
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
