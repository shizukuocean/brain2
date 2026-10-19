package brain2

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ConnectionProtocolVersion = "0.3"

type ConnectionStatus string

const (
	ConnectionPrepared          ConnectionStatus = "prepared"
	ConnectionRegistered        ConnectionStatus = "registered"
	ConnectionAwaitingHandshake ConnectionStatus = "awaiting_handshake"
	ConnectionConnected         ConnectionStatus = "connected"
	ConnectionFailed            ConnectionStatus = "failed"
)

type AgentDefinition struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Surface       string `json:"surface"`
	SupportLevel  string `json:"support_level"`
	QuickConnect  bool   `json:"quick_connect"`
	PromptConnect bool   `json:"prompt_connect"`
}

var supportedAgents = []AgentDefinition{
	{ID: "codex", Name: "Codex CLI", Surface: "cli", SupportLevel: "first-class", QuickConnect: true, PromptConnect: true},
	{ID: "claude-code", Name: "Claude Code", Surface: "cli", SupportLevel: "first-class", QuickConnect: true, PromptConnect: true},
	{ID: "hermes", Name: "Hermes", Surface: "cli", SupportLevel: "first-class-preview", QuickConnect: true, PromptConnect: true},
	{ID: "codex-desktop", Name: "Codex Desktop", Surface: "desktop", SupportLevel: "desktop-assisted", QuickConnect: true, PromptConnect: true},
	{ID: "claude-desktop", Name: "Claude Desktop", Surface: "desktop", SupportLevel: "desktop-assisted", QuickConnect: true, PromptConnect: true},
}

func SupportedAgents() []AgentDefinition {
	result := make([]AgentDefinition, len(supportedAgents))
	copy(result, supportedAgents)
	return result
}

func AgentDefinitionByID(agentID string) (AgentDefinition, bool) {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	for _, definition := range supportedAgents {
		if definition.ID == agentID {
			return definition, true
		}
	}
	return AgentDefinition{}, false
}

type ConnectionTask struct {
	SchemaVersion   string `json:"schema_version"`
	ProtocolVersion string `json:"protocol_version"`
	ConnectionID    string `json:"connection_id"`
	Nonce           string `json:"nonce"`
	AgentID         string `json:"agent_id"`
	DeviceID        string `json:"device_id"`
	Mode            string `json:"mode"`
	Brain2Root      string `json:"brain2_root"`
	WorkspaceID     string `json:"workspace_id"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
	LocalReady      bool   `json:"local_ready"`
	WorkIntent      string `json:"work_intent,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	ProjectName     string `json:"project_name,omitempty"`
	ProjectGoal     string `json:"project_goal,omitempty"`
	SkillSource     string `json:"skill_source,omitempty"`
}

type ConnectionPromptOptions struct {
	DeviceID     string
	LocalReady   bool
	WorkIntent   string
	ProjectID    string
	ProjectName  string
	ProjectGoal  string
	SkillSource  string
	InstallGuide string
}

type HandshakeInput struct {
	ConnectionID    string   `json:"connection_id"`
	Nonce           string   `json:"nonce"`
	AgentID         string   `json:"agent_id"`
	AgentVersion    string   `json:"agent_version"`
	Surface         string   `json:"surface"`
	ProtocolVersion string   `json:"protocol_version"`
	ReadFiles       []string `json:"read_files"`
	HealthStatus    string   `json:"health_status"`
}

type ConnectionReceipt struct {
	SchemaVersion   string   `json:"schema_version"`
	ProtocolVersion string   `json:"protocol_version"`
	ConnectionID    string   `json:"connection_id"`
	AgentID         string   `json:"agent_id"`
	DeviceID        string   `json:"device_id"`
	AgentVersion    string   `json:"agent_version"`
	Surface         string   `json:"surface"`
	Brain2Root      string   `json:"brain2_root"`
	WorkspaceID     string   `json:"workspace_id"`
	ReadFiles       []string `json:"read_files"`
	HealthStatus    string   `json:"health_status"`
	Status          string   `json:"status"`
	ConnectedAt     string   `json:"connected_at"`
}

func PrepareConnection(root, agentID, mode string) (ConnectionTask, string, error) {
	return PrepareConnectionWithOptions(root, agentID, mode, ConnectionPromptOptions{DeviceID: "A"})
}

func PrepareConnectionWithOptions(root, agentID, mode string, options ConnectionPromptOptions) (ConnectionTask, string, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ConnectionTask{}, "", err
	}
	definition, ok := AgentDefinitionByID(agentID)
	if !ok {
		return ConnectionTask{}, "", fmt.Errorf("不支持的 Agent %q", agentID)
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "quick" && mode != "prompt" {
		return ConnectionTask{}, "", errors.New("连接模式必须是 quick 或 prompt")
	}
	if mode == "quick" && !definition.QuickConnect {
		return ConnectionTask{}, "", fmt.Errorf("%s 当前需要 Prompt / Desktop 扩展连接", definition.Name)
	}
	deviceID := strings.TrimSpace(options.DeviceID)
	if deviceID == "" {
		deviceID = "A"
	}
	deviceID, err = NormalizeDeviceID(deviceID)
	if err != nil {
		return ConnectionTask{}, "", err
	}
	if err := PrepareRuntimeForDevice(root, deviceID, definition.ID); err != nil {
		return ConnectionTask{}, "", err
	}
	connectionID, err := randomHex(12)
	if err != nil {
		return ConnectionTask{}, "", err
	}
	nonce, err := randomHex(24)
	if err != nil {
		return ConnectionTask{}, "", err
	}
	now := time.Now().UTC()
	workspaceID, err := workspaceInstanceID(root)
	if err != nil {
		return ConnectionTask{}, "", err
	}
	task := ConnectionTask{
		SchemaVersion: schemaVersion, ProtocolVersion: ConnectionProtocolVersion,
		ConnectionID: connectionID, Nonce: nonce, AgentID: definition.ID, DeviceID: deviceID, Mode: mode,
		Brain2Root: root, WorkspaceID: workspaceID, Status: string(ConnectionAwaitingHandshake),
		CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339Nano),
		LocalReady: options.LocalReady, WorkIntent: strings.TrimSpace(options.WorkIntent),
		ProjectID: strings.TrimSpace(options.ProjectID), ProjectName: strings.TrimSpace(options.ProjectName),
		ProjectGoal: strings.TrimSpace(options.ProjectGoal), SkillSource: strings.TrimSpace(options.SkillSource),
	}
	dir := filepath.Join(root, ".brain2", "connections", connectionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ConnectionTask{}, "", err
	}
	if err := writeJSON(filepath.Join(dir, "task.json"), task); err != nil {
		return ConnectionTask{}, "", err
	}
	if err := writeJSON(filepath.Join(dir, "receipt.schema.json"), receiptSchema()); err != nil {
		return ConnectionTask{}, "", err
	}
	prompt := connectionPrompt(task, definition, options.InstallGuide)
	if err := atomicWrite(filepath.Join(dir, "PROMPT.md"), []byte(prompt), 0o644); err != nil {
		return ConnectionTask{}, "", err
	}
	if err := updateAgentForDevice(root, deviceID, definition.ID, func(agent *Agent) {
		agent.ConnectionID = connectionID
		if mode == "quick" {
			agent.ConnectionStatus = string(ConnectionAwaitingHandshake)
			agent.ConnectionMode = mode
			agent.AdapterLoaded = false
			agent.LastError = ""
			return
		}
		// 生成 Prompt 不应撤销已经完成的本地快速接入。未安装时只记录为已准备，
		// 真正的连接状态仍由 Agent 在实际工作会话中提交结构化回执后更新。
		if agent.ConnectionStatus != string(ConnectionRegistered) && agent.ConnectionStatus != string(ConnectionConnected) {
			agent.ConnectionStatus = string(ConnectionPrepared)
			agent.ConnectionMode = mode
			agent.AdapterLoaded = false
			agent.LastError = ""
		}
	}); err != nil {
		return ConnectionTask{}, "", err
	}
	return task, prompt, nil
}

func LoadConnectionTask(root, connectionID string) (ConnectionTask, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ConnectionTask{}, err
	}
	if !validToken(connectionID) {
		return ConnectionTask{}, errors.New("无效的 connection_id")
	}
	var task ConnectionTask
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "connections", connectionID, "task.json"))
	if err != nil {
		return task, err
	}
	if err := json.Unmarshal(data, &task); err != nil {
		return task, err
	}
	return task, nil
}

func CompleteConnection(root string, input HandshakeInput) (ConnectionReceipt, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ConnectionReceipt{}, err
	}
	task, err := LoadConnectionTask(root, input.ConnectionID)
	if err != nil {
		return ConnectionReceipt{}, err
	}
	if strings.TrimSpace(task.DeviceID) == "" {
		cfg, _ := LoadConfig(root)
		task.DeviceID = InferLegacyDeviceID(cfg.Brain2Root, "A")
	}
	if time.Now().UTC().After(mustParseTime(task.ExpiresAt)) {
		return ConnectionReceipt{}, errors.New("连接任务已过期，请在 Launcher 中重新发起")
	}
	if subtle.ConstantTimeCompare([]byte(task.Nonce), []byte(strings.TrimSpace(input.Nonce))) != 1 {
		return ConnectionReceipt{}, errors.New("连接 nonce 校验失败")
	}
	if task.AgentID != strings.ToLower(strings.TrimSpace(input.AgentID)) {
		return ConnectionReceipt{}, errors.New("Agent 身份与连接任务不一致")
	}
	if input.ProtocolVersion != ConnectionProtocolVersion {
		return ConnectionReceipt{}, fmt.Errorf("协议版本不匹配：需要 %s", ConnectionProtocolVersion)
	}
	healthStatus := strings.ToLower(strings.TrimSpace(input.HealthStatus))
	if healthStatus != "healthy" && healthStatus != "needs_attention" {
		return ConnectionReceipt{}, errors.New("Brain2 健康检查未通过")
	}
	if len(input.ReadFiles) == 0 {
		return ConnectionReceipt{}, errors.New("回执缺少已读取入口证据")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	receipt := ConnectionReceipt{
		SchemaVersion: schemaVersion, ProtocolVersion: ConnectionProtocolVersion,
		ConnectionID: task.ConnectionID, AgentID: task.AgentID, DeviceID: task.DeviceID,
		AgentVersion: strings.TrimSpace(input.AgentVersion), Surface: strings.TrimSpace(input.Surface),
		Brain2Root: root, WorkspaceID: task.WorkspaceID, ReadFiles: uniqueStrings(input.ReadFiles),
		HealthStatus: healthStatus, Status: string(ConnectionConnected), ConnectedAt: now,
	}
	dir := filepath.Join(root, ".brain2", "connections", task.ConnectionID)
	if err := writeJSON(filepath.Join(dir, "receipt.json"), receipt); err != nil {
		return ConnectionReceipt{}, err
	}
	task.Status = string(ConnectionConnected)
	if err := writeJSON(filepath.Join(dir, "task.json"), task); err != nil {
		return ConnectionReceipt{}, err
	}
	if err := updateAgentForDevice(root, task.DeviceID, task.AgentID, func(agent *Agent) {
		agent.ConnectionStatus = string(ConnectionConnected)
		agent.ConnectionMode = task.Mode
		agent.ConnectionID = task.ConnectionID
		agent.AdapterLoaded = true
		agent.ConnectedAt = now
		agent.LastError = ""
	}); err != nil {
		return ConnectionReceipt{}, err
	}
	return receipt, nil
}

func MarkConnectionFailed(root, agentID, connectionID, message string) error {
	return MarkConnectionFailedForDevice(root, "A", agentID, connectionID, message)
}

func MarkConnectionFailedForDevice(root, deviceID, agentID, connectionID, message string) error {
	return updateAgentForDevice(root, deviceID, agentID, func(agent *Agent) {
		agent.ConnectionStatus = string(ConnectionFailed)
		agent.ConnectionID = connectionID
		agent.AdapterLoaded = false
		agent.LastError = strings.TrimSpace(message)
	})
}

func MarkAgentRegistered(root, agentID, mode string) error {
	return MarkAgentRegisteredForDevice(root, "A", agentID, mode)
}

func MarkAgentRegisteredForDevice(root, deviceID, agentID, mode string) error {
	return updateAgentForDevice(root, deviceID, agentID, func(agent *Agent) {
		if agent.ConnectionStatus == string(ConnectionConnected) {
			// Refreshing a Launcher-managed Skill or MCP Host is a local
			// maintenance operation. Keep an already validated Agent seat and
			// its receipt instead of silently downgrading it to registered.
			agent.ConnectionMode = strings.TrimSpace(mode)
			agent.LastError = ""
			return
		}
		agent.ConnectionStatus = string(ConnectionRegistered)
		agent.ConnectionMode = strings.TrimSpace(mode)
		agent.ConnectionID = ""
		agent.AdapterLoaded = false
		agent.ConnectedAt = ""
		agent.LastError = ""
	})
}

// MarkAgentRestartRequired invalidates the previous runtime receipt after a
// Launcher-managed Skill, MCP Host, or Agent configuration update. Installation
// success is local evidence; only a fresh handshake can prove the restarted
// Agent actually loaded the new connector.
func MarkAgentRestartRequiredForDevice(root, deviceID, agentID, mode string) error {
	return updateAgentForDevice(root, deviceID, agentID, func(agent *Agent) {
		agent.ConnectionStatus = string(ConnectionRegistered)
		agent.ConnectionMode = strings.TrimSpace(mode)
		agent.ConnectionID = ""
		agent.AdapterLoaded = false
		agent.ConnectedAt = ""
		agent.LastError = "连接器已更新，等待重启 Agent 并重新验证"
	})
}

func MarkVerificationFailed(root, agentID, connectionID, message string) error {
	return MarkVerificationFailedForDevice(root, "A", agentID, connectionID, message)
}

func MarkVerificationFailedForDevice(root, deviceID, agentID, connectionID, message string) error {
	return updateAgentForDevice(root, deviceID, agentID, func(agent *Agent) {
		agent.ConnectionStatus = string(ConnectionRegistered)
		agent.ConnectionMode = "quick"
		agent.ConnectionID = connectionID
		agent.AdapterLoaded = false
		agent.ConnectedAt = ""
		agent.LastError = strings.TrimSpace(message)
	})
}

func updateAgent(root, agentID string, mutate func(*Agent)) error {
	return updateAgentForDevice(root, "A", agentID, mutate)
}

func updateAgentForDevice(root, deviceID, agentID string, mutate func(*Agent)) error {
	definition, ok := AgentDefinitionByID(agentID)
	if !ok {
		return fmt.Errorf("不支持的 Agent %q", agentID)
	}
	deviceID, err := NormalizeDeviceID(deviceID)
	if err != nil {
		return err
	}
	reg, err := LoadAgents(root)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	index := -1
	for i := range reg.Agents {
		if reg.Agents[i].DeviceID == deviceID && reg.Agents[i].ID == definition.ID {
			index = i
			break
		}
	}
	if index < 0 {
		reg.Agents = append(reg.Agents, Agent{
			DeviceID: deviceID, ID: definition.ID, Name: definition.Name, Surface: definition.Surface,
			SupportLevel: definition.SupportLevel, RuntimeVersion: "0.3.0",
			AdapterVersion: "0.3.0",
		})
		index = len(reg.Agents) - 1
	}
	mutate(&reg.Agents[index])
	reg.Agents[index].UpdatedAt = now
	return writeJSON(filepath.Join(root, ".brain2", "agents.json"), reg)
}

func connectionPrompt(task ConnectionTask, definition AgentDefinition, installGuide string) string {
	localInstruction := `Launcher 尚未确认本 Agent 的本地接入。先使用你原生的文件读取与命令执行能力读取下方 Skill 来源，并按安装说明完成 Skill 与 brain2-meow MCP 注册；然后再调用 MCP。`
	if task.LocalReady {
		localInstruction = `Launcher 已完成本地快速接入。不要重复安装 Skill 或 MCP，直接使用 brain2-meow MCP。`
	}
	workInstruction := "完成连接后，读取 Brain2 总控与当前上下文，确认当前任务后再行动。"
	activationInstruction := `输入“读取大脑，告诉我当前上下文和下一步”即可开始。`
	reportProject := "未指定"
	switch task.WorkIntent {
	case "existing_project":
		workInstruction = fmt.Sprintf("完成连接后进入已有项目：%s（project_id=%s）。先读取项目入口和当前进度，再继续工作。", task.ProjectName, task.ProjectID)
		activationInstruction = fmt.Sprintf(`输入“读取大脑，进入「%s」”即可开始。`, task.ProjectName)
		reportProject = task.ProjectName
	case "new_project":
		workInstruction = "完成连接后先与用户沟通项目目标、已有材料和本地目录；由你提出最小项目结构。获得用户确认后，再通过 Brain2 受控工具建立项目上下文。Launcher 不替用户定义目标，也不要自动整理旧内容。"
		activationInstruction = `输入“读取大脑，我想开始一个新项目”即可开始。`
		reportProject = "等待与用户确认"
	}
	return fmt.Sprintf(`你正在通过 Meow Launcher 进入本地 Brain2。这是一份由用户在 Launcher 中选择后生成的工作入口，不是通用聊天提示。

连接参数：
- device_id: %s
- agent_id: %s
- connection_id: %s
- nonce: %s
- protocol_version: %s
- brain2_root: %s
- workspace_id: %s
- local_ready: %t
- work_intent: %s
- project_id: %s
- project_name: %s
- project_goal: %s
- skill_source: %s

本地接入状态：
%s

安装说明（仅在 local_ready=false 时执行）：
%s

必须按顺序执行：
1. 确保 brain2-connect Skill 与 brain2-meow MCP 已可用；已安装时不要重复修改配置。
2. 调用 brain2_get_bootstrap，传入 connection_id、nonce、agent_id。
3. 阅读工具返回的 Brain2 固定入口和治理规则，不要只复述文件名。
4. 调用 brain2_health，传入同一 connection_id；核对返回的 workspace_id 与 brain2_root 必须和本任务一致。healthy 可继续，needs_attention 需如实报告非阻塞检查项，critical 不得完成成功握手。
5. 调用 brain2_complete_handshake，提交同一 connection_id、nonce、agent_id、你的版本与 surface=%s。
6. 只有 complete_handshake 成功后才可以声明已连接；失败时返回真实错误，不得伪造回执。
7. %s

完成反馈：
- 回复前必须实际检查 Skill 文件、MCP 注册、五个 Brain2 工具、健康状态与握手结果，不能只根据安装命令的退出码判断成功。
- “Skill 已安装”和“当前会话已加载”必须分开报告；如果新装 Skill 需要重启 Agent 才能加载，明确写“需要重启后加载”。
- 不要在回复中暴露 nonce、连接密钥或 Brain2 私有正文。
- 最后必须使用以下固定格式回复用户；失败或部分成功时，在“注意事项”写明失败组件和下一步：

Brain2 接入结果：成功 / 部分成功 / 失败
Agent：%s (%s)
Skill：brain2-connect 已安装；已加载 / 需要重启后加载 / 未加载
MCP：brain2-meow 已注册；五个工具检查通过 / 未通过
Brain2：%s
握手：已完成 / 未完成
当前项目：%s
下次使用：%s
注意事项：无 / <失败组件、真实错误与下一步>
`, task.DeviceID, definition.ID, task.ConnectionID, task.Nonce, task.ProtocolVersion, task.Brain2Root, task.WorkspaceID, task.LocalReady,
		task.WorkIntent, task.ProjectID, task.ProjectName, task.ProjectGoal, task.SkillSource,
		localInstruction, strings.TrimSpace(installGuide), definition.Surface, workInstruction,
		definition.Name, task.DeviceID, task.Brain2Root, reportProject, activationInstruction)
}

func receiptSchema() map[string]any {
	return map[string]any{
		"$schema":  "https://json-schema.org/draft/2020-12/schema",
		"type":     "object",
		"required": []string{"connection_id", "device_id", "agent_id", "status", "connected_at"},
		"properties": map[string]any{
			"connection_id": map[string]any{"type": "string"},
			"device_id":     map[string]any{"enum": []string{"A", "B"}},
			"agent_id":      map[string]any{"type": "string"},
			"status":        map[string]any{"const": string(ConnectionConnected)},
			"connected_at":  map[string]any{"type": "string"},
		},
	}
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func validToken(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func mustParseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
