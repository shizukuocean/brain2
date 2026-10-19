package brain2

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meow-launcher/internal/productregistry"
)

func TestConnectionRequiresValidatedReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	task, prompt, err := PrepareConnection(root, "codex", "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != string(ConnectionAwaitingHandshake) || prompt == "" {
		t.Fatalf("unexpected task: %#v", task)
	}
	if _, err := CompleteConnection(root, HandshakeInput{
		ConnectionID: task.ConnectionID, Nonce: "wrong", AgentID: "codex",
		ProtocolVersion: ConnectionProtocolVersion, HealthStatus: "healthy", ReadFiles: []string{"NAV.md"},
	}); err == nil {
		t.Fatal("expected nonce mismatch to fail")
	}
	receipt, err := CompleteConnection(root, HandshakeInput{
		ConnectionID: task.ConnectionID, Nonce: task.Nonce, AgentID: "codex",
		AgentVersion: "test", Surface: "cli", ProtocolVersion: ConnectionProtocolVersion,
		HealthStatus: "healthy", ReadFiles: []string{"NAV.md", "NAV.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != string(ConnectionConnected) || len(receipt.ReadFiles) != 1 {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	agents, err := LoadAgents(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents.Agents) != 1 || !agents.Agents[0].AdapterLoaded || agents.Agents[0].ConnectionStatus != string(ConnectionConnected) {
		t.Fatalf("unexpected connected registry: %#v", agents)
	}
}

func TestConnectionAllowsNonCriticalProjectManifestWarning(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	task, _, err := PrepareConnection(root, "codex", "prompt")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := CompleteConnection(root, HandshakeInput{
		ConnectionID: task.ConnectionID, Nonce: task.Nonce, AgentID: "codex",
		AgentVersion: "test", Surface: "cli", ProtocolVersion: ConnectionProtocolVersion,
		HealthStatus: "needs_attention", ReadFiles: []string{"NAV.md"},
	})
	if err != nil {
		t.Fatalf("non-critical project warning should not block Agent handshake: %v", err)
	}
	if receipt.HealthStatus != "needs_attention" || receipt.Status != string(ConnectionConnected) {
		t.Fatalf("unexpected warning receipt: %#v", receipt)
	}
}

func TestMCPBootstrapCompletesHandshake(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	task, _, err := PrepareConnection(root, "claude-code", "quick")
	if err != nil {
		t.Fatal(err)
	}
	requests := []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
		{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "brain2_get_bootstrap", "arguments": map[string]any{"connection_id": task.ConnectionID, "nonce": task.Nonce, "agent_id": task.AgentID}}},
		{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "brain2_health", "arguments": map[string]any{"connection_id": task.ConnectionID}}},
		{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "brain2_complete_handshake", "arguments": map[string]any{"connection_id": task.ConnectionID, "nonce": task.Nonce, "agent_id": task.AgentID, "agent_version": "test", "surface": "cli"}}},
	}
	var input bytes.Buffer
	for _, request := range requests {
		if err := json.NewEncoder(&input).Encode(request); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := ServeMCP(root, &input, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".brain2", "connections", task.ConnectionID, "receipt.json")); err != nil {
		t.Fatalf("receipt missing: %v\nMCP output:\n%s", err, output.String())
	}
}

func TestRegistryMCPRoutesEachConnectionToItsWorkspace(t *testing.T) {
	base := t.TempDir()
	registryPath := filepath.Join(base, "launcher", "registry.json")
	roots := []string{filepath.Join(base, "Brain A"), filepath.Join(base, "Brain B")}
	var task ConnectionTask
	for index, root := range roots {
		if err := Initialize(root); err != nil {
			t.Fatal(err)
		}
		cfg, err := EnsureInstanceID(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := productregistry.UpsertBrain2(registryPath, root, cfg.InstanceID); err != nil {
			t.Fatal(err)
		}
		if index == 1 {
			task, _, err = PrepareConnection(root, "codex", "prompt")
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	requests := []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "brain2_get_bootstrap", "arguments": map[string]any{"connection_id": task.ConnectionID, "nonce": task.Nonce, "agent_id": task.AgentID}}},
		{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "brain2_health", "arguments": map[string]any{"connection_id": task.ConnectionID}}},
	}
	var input bytes.Buffer
	for _, request := range requests {
		if err := json.NewEncoder(&input).Encode(request); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := ServeMCPRegistry(registryPath, &input, &output); err != nil {
		t.Fatal(err)
	}
	decodedOutput := strings.ReplaceAll(output.String(), `\\`, `\`)
	if !strings.Contains(decodedOutput, filepath.Clean(roots[1])) || strings.Contains(decodedOutput, filepath.Clean(roots[0])+`\"`) {
		t.Fatalf("registry host did not route to Brain B:\n%s", output.String())
	}
	if !strings.Contains(output.String(), `"mcp_host_version":"`+MCPHostVersion+`"`) {
		t.Fatalf("health identity missing from registry response:\n%s", output.String())
	}
}

func TestRegisteredStateSurvivesOnlineVerificationFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntime(root, "codex"); err != nil {
		t.Fatal(err)
	}
	if err := MarkAgentRegistered(root, "codex", "quick"); err != nil {
		t.Fatal(err)
	}
	task, _, err := PrepareConnection(root, "codex", "quick")
	if err != nil {
		t.Fatal(err)
	}
	if err := MarkVerificationFailed(root, "codex", task.ConnectionID, "network timeout"); err != nil {
		t.Fatal(err)
	}
	agents, err := LoadAgents(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents.Agents) != 1 || agents.Agents[0].ConnectionStatus != string(ConnectionRegistered) || agents.Agents[0].AdapterLoaded {
		t.Fatalf("verification failure must preserve only local registration: %#v", agents)
	}
	if agents.Agents[0].LastError != "network timeout" {
		t.Fatalf("verification error was not preserved: %#v", agents.Agents[0])
	}
}

func TestLocalHostRefreshPreservesConnectedAgent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntime(root, "claude-code"); err != nil {
		t.Fatal(err)
	}
	connectedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := updateAgent(root, "claude-code", func(agent *Agent) {
		agent.ConnectionStatus = string(ConnectionConnected)
		agent.AdapterLoaded = true
		agent.ConnectedAt = connectedAt
		agent.ConnectionID = "receipt-kept"
	}); err != nil {
		t.Fatal(err)
	}
	if err := MarkAgentRegistered(root, "claude-code", "quick"); err != nil {
		t.Fatal(err)
	}
	agents, err := LoadAgents(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents.Agents) != 1 {
		t.Fatalf("unexpected agents: %#v", agents.Agents)
	}
	agent := agents.Agents[0]
	if agent.ConnectionStatus != string(ConnectionConnected) || !agent.AdapterLoaded || agent.ConnectedAt != connectedAt || agent.ConnectionID != "receipt-kept" {
		t.Fatalf("local Host refresh downgraded validated Agent: %#v", agent)
	}
}

func TestPromptPreparationPreservesQuickInstallAndCarriesWorkIntent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := PrepareRuntime(root, "codex"); err != nil {
		t.Fatal(err)
	}
	if err := MarkAgentRegistered(root, "codex", "quick"); err != nil {
		t.Fatal(err)
	}
	task, prompt, err := PrepareConnectionWithOptions(root, "codex", "prompt", ConnectionPromptOptions{
		LocalReady: true, WorkIntent: "new_project", ProjectName: "Launcher 接入改造",
		ProjectGoal: "验证双轨 Prompt", SkillSource: `D:\Brain2\.brain2\runtime\bootstrap\brain2-connect\SKILL.md`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !task.LocalReady || task.WorkIntent != "new_project" || task.ProjectName != "Launcher 接入改造" {
		t.Fatalf("work intent missing from task: %#v", task)
	}
	for _, required := range []string{
		"不要重复安装 Skill 或 MCP",
		"Launcher 接入改造",
		"Brain2 接入结果：成功 / 部分成功 / 失败",
		"Skill：brain2-connect 已安装；已加载 / 需要重启后加载 / 未加载",
		"MCP：brain2-meow 已注册；五个工具检查通过 / 未通过",
		"先与用户沟通项目目标",
		"Launcher 不替用户定义目标",
		"读取大脑，我想开始一个新项目",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("generated work Prompt is missing %q:\n%s", required, prompt)
		}
	}
	agents, err := LoadAgents(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents.Agents) != 1 || agents.Agents[0].ConnectionStatus != string(ConnectionRegistered) {
		t.Fatalf("generating a Prompt must preserve quick install state: %#v", agents)
	}
}
