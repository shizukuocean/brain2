package agentconnect

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"meow-launcher/internal/brain2"
	"meow-launcher/internal/productregistry"
)

func TestRegisterMCPBuildsSupportedCLICommands(t *testing.T) {
	tests := []struct {
		id       string
		contains string
	}{
		{id: "codex", contains: "mcp add brain2-meow -- C:\\Launcher.exe mcp-serve --registry D:\\Brain2"},
		{id: "claude-code", contains: "mcp add --scope user brain2-meow -- C:\\Launcher.exe mcp-serve --registry D:\\Brain2"},
		{id: "hermes", contains: "mcp add brain2-meow --command C:\\Launcher.exe --args mcp-serve --registry D:\\Brain2"},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			calls := []string{}
			registered := false
			runner := func(_ context.Context, executable string, args []string, stdin string) ([]byte, error) {
				call := strings.Join(append([]string{executable}, args...), " ")
				calls = append(calls, call)
				if strings.Contains(call, " mcp get ") {
					return nil, errors.New("not registered")
				}
				if strings.HasSuffix(call, " mcp list") {
					if test.id == "hermes" && registered {
						return []byte("brain2-meow  all  enabled"), nil
					}
					return nil, errors.New("not registered")
				}
				if test.id == "hermes" && strings.Contains(call, " mcp add ") {
					if stdin != "y\n" {
						t.Fatalf("Hermes registration must confirm tool enablement, stdin=%q", stdin)
					}
					registered = true
				}
				return []byte("ok"), nil
			}
			detection := Detection{ID: test.id, Name: test.id, Executable: test.id + ".exe", Detected: true}
			if err := registerMCP(context.Background(), runner, detection, `D:\Brain2`, `C:\Launcher.exe`, false); err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(calls, "\n")
			if !strings.Contains(joined, test.contains) {
				t.Fatalf("missing command %q in:\n%s", test.contains, joined)
			}
		})
	}
}

func TestRegisterHermesRejectsCancelledFalsePositive(t *testing.T) {
	listCalls := 0
	runner := func(_ context.Context, _ string, args []string, stdin string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.HasSuffix(joined, "mcp list") {
			listCalls++
			if listCalls == 1 {
				return []byte("No MCP servers configured."), nil
			}
			return []byte("No MCP servers configured."), nil
		}
		if strings.Contains(joined, "mcp add") {
			if stdin != "y\n" {
				t.Fatalf("expected automatic confirmation, got %q", stdin)
			}
			return []byte("Cancelled."), nil
		}
		return []byte("ok"), nil
	}
	detection := Detection{ID: "hermes", Name: "Hermes", Executable: "hermes.exe", Detected: true}
	err := registerMCP(context.Background(), runner, detection, `D:\Brain2`, `C:\Launcher.exe`, false)
	if err == nil || !strings.Contains(err.Error(), "注册未生效") {
		t.Fatalf("expected cancelled registration to fail closed, got %v", err)
	}
}

func TestRegisterMCPReplacesOwnedCodexHostWhenBinaryVersionChanges(t *testing.T) {
	calls := []string{}
	runner := func(_ context.Context, executable string, args []string, _ string) ([]byte, error) {
		call := strings.Join(append([]string{executable}, args...), " ")
		calls = append(calls, call)
		if strings.Contains(call, " mcp get ") {
			return []byte(`{"transport":{"command":"D:\\MeowLauncher\\runtime\\brain2-mcp-host-old.exe","args":["mcp-serve","--root","D:\\Brain2"]}}`), nil
		}
		return []byte("ok"), nil
	}
	detection := Detection{ID: "codex", Name: "Codex CLI", Executable: "codex.exe", Detected: true}
	if err := registerMCP(context.Background(), runner, detection, `D:\Brain2`, `D:\MeowLauncher\runtime\brain2-mcp-host-new.exe`, false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "mcp remove brain2-meow") || !strings.Contains(joined, "mcp add brain2-meow -- D:\\MeowLauncher\\runtime\\brain2-mcp-host-new.exe") {
		t.Fatalf("expected old owned Host to be replaced, calls:\n%s", joined)
	}
}

func TestRegisterMCPRebindsDifferentRootOnlyWithExplicitPermission(t *testing.T) {
	calls := []string{}
	runner := func(_ context.Context, executable string, args []string, _ string) ([]byte, error) {
		call := strings.Join(append([]string{executable}, args...), " ")
		calls = append(calls, call)
		if strings.Contains(call, " mcp get ") {
			return []byte(`{"transport":{"command":"D:\\MeowLauncher\\runtime\\brain2-mcp-host-old.exe","args":["mcp-serve","--registry","E:\\OtherLauncher\\registry.json"]}}`), nil
		}
		return []byte("ok"), nil
	}
	detection := Detection{ID: "codex", Name: "Codex CLI", Executable: "codex.exe", Detected: true}
	if err := registerMCP(context.Background(), runner, detection, `D:\ProductTest`, `D:\MeowLauncher\runtime\brain2-mcp-host-v0.4.0.exe`, false); err == nil {
		t.Fatal("different root was replaced without explicit permission")
	}
	calls = nil
	if err := registerMCP(context.Background(), runner, detection, `D:\ProductTest`, `D:\MeowLauncher\runtime\brain2-mcp-host-v0.4.0.exe`, true); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "mcp remove brain2-meow") || !strings.Contains(joined, `--registry D:\ProductTest`) {
		t.Fatalf("explicit rebind did not replace the Launcher registry:\n%s", joined)
	}
}

func TestAuditMCPRegistrationClassifiesOwnedHostVersions(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "current",
			output: `{"name":"brain2-meow","transport":{"command":"D:\\MeowLauncher\\runtime\\brain2-mcp-host-v0.4.0.exe","args":["mcp-serve","--registry","D:\\Brain2"]}}`,
			want:   "current",
		},
		{
			name:   "update available",
			output: `{"name":"brain2-meow","transport":{"command":"D:\\MeowLauncher\\runtime\\brain2-mcp-host-oldhash.exe","args":["mcp-serve","--root","D:\\Brain2"]}}`,
			want:   "update_available",
		},
		{
			name:   "root conflict",
			output: `{"name":"brain2-meow","transport":{"command":"D:\\MeowLauncher\\runtime\\brain2-mcp-host-oldhash.exe","args":["mcp-serve","--registry","E:\\OtherLauncher\\registry.json"]}}`,
			want:   "conflict",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := func(context.Context, string, []string, string) ([]byte, error) {
				return []byte(test.output), nil
			}
			detection := Detection{ID: "codex", Executable: "codex.exe", Detected: true}
			got, _, _ := auditMCPRegistration(context.Background(), runner, detection, `D:\Brain2`, "brain2-mcp-host-v0.4.0")
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRegisterHermesReplacesEnabledOwnedOldHost(t *testing.T) {
	calls := []string{}
	removed := false
	added := false
	runner := func(_ context.Context, executable string, args []string, stdin string) ([]byte, error) {
		call := strings.Join(append([]string{executable}, args...), " ")
		calls = append(calls, call)
		if strings.Contains(call, `sh -lc cat "$HOME/.hermes/config.yaml"`) {
			return []byte("mcp_servers:\n  brain2-meow:\n    command: /home/hermes/.hermes/bin/brain2-mcp-host-oldhash\n    args:\n      - mcp-serve\n      - --root\n      - /mnt/d/Brain2\n    enabled: true\n"), nil
		}
		if strings.HasSuffix(call, "mcp list") {
			if added || !removed {
				return []byte("brain2-meow  all  enabled"), nil
			}
			return []byte("No MCP servers configured."), nil
		}
		if strings.Contains(call, "mcp remove brain2-meow") {
			if stdin != "y\n" {
				t.Fatalf("expected Hermes removal confirmation, got %q", stdin)
			}
			removed = true
			return []byte("removed"), nil
		}
		if strings.Contains(call, "mcp add brain2-meow") {
			added = true
			return []byte("added"), nil
		}
		return []byte("ok"), nil
	}
	detection := Detection{
		ID: "hermes", Name: "Hermes", Executable: "wsl.exe", Detected: true,
		PrefixArgs: []string{"-d", "Ubuntu-Hermes", "--", "/usr/local/bin/hermes"},
	}
	if err := registerMCP(context.Background(), runner, detection, `D:\Brain2`, "/home/hermes/.hermes/bin/brain2-mcp-host-newhash", false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !removed || !added || !strings.Contains(joined, "brain2-mcp-host-newhash") {
		t.Fatalf("expected managed Hermes Host replacement, calls:\n%s", joined)
	}
}

func TestVersionedHostNameUsesSemanticMCPVersion(t *testing.T) {
	if got, want := versionedHostName(), "brain2-mcp-host-v"+MCPHostVersion+".exe"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindPackagedCodexExecutableUsesAppXResources(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	executable := filepath.Join(newRoot, "app", "resources", "codex.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findPackagedCodexExecutable([]string{oldRoot, newRoot}); got != executable {
		t.Fatalf("packaged Codex executable = %q, want %q", got, executable)
	}
}

func TestEmbeddedHostMatchesCurrentMultiDeviceWorkspaceRouting(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "Brain2")
	if err := brain2.Initialize(root); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, ".brain2", "config.json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	config["brain2_root"] = `D:\Brain2`
	config["device_name"] = "OLD-SYNCED-MACHINE"
	configData, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, append(configData, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	task, _, err := brain2.PrepareConnectionWithOptions(root, "codex-desktop", "quick", brain2.ConnectionPromptOptions{DeviceID: "A"})
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(temp, "registry.json")
	workspaceConfig, err := brain2.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := productregistry.UpsertBrain2(registryPath, root, workspaceConfig.InstanceID); err != nil {
		t.Fatal(err)
	}
	asset, err := runtimeAssets.ReadFile("assets/brain2-mcp-host-windows-amd64.exe")
	if err != nil {
		t.Fatal(err)
	}
	hostPath, _, err := installStableHostDataTo(asset, filepath.Join(temp, "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"embedded-host-regression","version":"1"}}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"brain2_get_bootstrap","arguments":{"connection_id":%q,"nonce":%q,"agent_id":%q}}}`, task.ConnectionID, task.Nonce, task.AgentID),
		"",
	}, "\n")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := runCommand(ctx, hostPath, []string{"mcp-serve", "--registry", registryPath}, input)
	if err != nil {
		t.Fatal(err)
	}
	value := string(output)
	if strings.Contains(value, "connection_task_not_found") || !strings.Contains(value, workspaceConfig.InstanceID) {
		t.Fatalf("embedded Host cannot route a workspace with synced legacy device hints:\n%s", value)
	}
	if !strings.Contains(value, `"version":"`+MCPHostVersion+`"`) {
		t.Fatalf("embedded Host does not report MCP version %s:\n%s", MCPHostVersion, value)
	}
}

func TestCodexDesktopRegistrationWritesUserConfigWithoutExecutingWindowsAppsBinary(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	original := "model = \"gpt-5\"\n\n[mcp_servers.node_repl]\ncommand = \"node.exe\"\nargs = []\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	runnerCalled := false
	runner := func(context.Context, string, []string, string) ([]byte, error) {
		runnerCalled = true
		return nil, errors.New("WindowsApps binary must not run")
	}
	detection := Detection{
		ID: "codex-desktop", Name: "Codex Desktop", Detected: true,
		Executable: `C:\Program Files\WindowsApps\OpenAI.Codex_test\app\resources\codex.exe`,
	}
	if err := registerMCP(context.Background(), runner, detection, `E:\MeowLauncher\profiles\memory-libraries.json`, `E:\MeowLauncher\runtime\brain2-mcp-host-v0.4.0.exe`, false); err != nil {
		t.Fatal(err)
	}
	if runnerCalled {
		t.Fatal("Codex Desktop registration attempted to execute the packaged binary")
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	value := string(updated)
	for _, expected := range []string{"[mcp_servers.node_repl]", "[mcp_servers.brain2-meow]", `brain2-mcp-host-v0.4.0.exe`, `memory-libraries.json`} {
		if !strings.Contains(value, expected) {
			t.Fatalf("updated config missing %q:\n%s", expected, value)
		}
	}
	backups, err := filepath.Glob(configPath + ".brain2-backup-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one safety backup, got %v (err=%v)", backups, err)
	}
}

func TestCodexDesktopRegistrationRejectsRegistryConflictWithoutChangingConfig(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	original := renderCodexBrain2Table(`D:\OtherLauncher\registry.json`, `D:\runtime\brain2-mcp-host-v0.3.0.exe`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	err := registerCodexDesktopMCP(`E:\MeowLauncher\profiles\memory-libraries.json`, `E:\runtime\brain2-mcp-host-v0.4.0.exe`, false)
	if err == nil || !strings.Contains(err.Error(), "另一套 Launcher") {
		t.Fatalf("expected a registry conflict, got %v", err)
	}
	unchanged, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(unchanged, original) {
		t.Fatalf("conflicting config changed:\n%s", unchanged)
	}
}

func TestCodexDesktopRegistrationFailsClosedForMalformedConfig(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	original := []byte("[mcp_servers.node_repl\ncommand = \"node.exe\"\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	err := registerCodexDesktopMCP(`E:\registry.json`, `E:\host.exe`, false)
	if err == nil || !strings.Contains(err.Error(), "无法解析") {
		t.Fatalf("expected a Chinese parse error, got %v", err)
	}
	unchanged, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(unchanged, original) {
		t.Fatal("malformed config was changed")
	}
}

func TestRestoreCodexDesktopRegistrationRemovesNewManagedTable(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("CODEX_HOME", codexHome)
	configPath := filepath.Join(codexHome, "config.toml")
	current := append([]byte("model = \"gpt-5\"\n"), renderCodexBrain2Table(`E:\registry.json`, `E:\host.exe`)...)
	if err := os.WriteFile(configPath, current, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restoreCodexDesktopMCPRegistration(nil); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), "brain2-meow") || !strings.Contains(string(updated), "model") {
		t.Fatalf("unexpected restored config:\n%s", updated)
	}
}

func TestConfiguredHostCommandSupportsPortableManagedPaths(t *testing.T) {
	windows := []byte(`{"transport":{"command":"C:\\Users\\Person Name\\AppData\\Local\\MeowLauncher\\runtime\\brain2-mcp-host-v0.4.0.exe","args":["mcp-serve","--registry","C:\\Users\\Person Name\\registry.json"]}}`)
	if got := configuredHostCommand(windows); got != `C:\Users\Person Name\AppData\Local\MeowLauncher\runtime\brain2-mcp-host-v0.4.0.exe` {
		t.Fatalf("unexpected Windows Host path: %q", got)
	}
	linux := []byte("command: /home/person/.local/share/meow/brain2-mcp-host-v0.4.0\n")
	if got := configuredHostCommand(linux); got != "/home/person/.local/share/meow/brain2-mcp-host-v0.4.0" {
		t.Fatalf("unexpected Linux Host path: %q", got)
	}
}

func TestConnectorBackupIsMachineLocalAndRollbackDiscoverable(t *testing.T) {
	machine := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machine)
	backup := ConnectorBackup{
		SchemaVersion: "0.1", AgentID: "codex", Status: "success",
		RegistryPath: filepath.Join(machine, "runtime", "brain2-registry.json"),
		PreviousHost: filepath.Join(machine, "runtime", "brain2-mcp-host-v0.4.0.exe"),
	}
	path := filepath.Join(machine, "runtime", "connector-backups", "codex", "last-update.json")
	if err := writeConnectorBackup(path, backup); err != nil {
		t.Fatal(err)
	}
	if !connectorRollbackAvailable("codex") {
		t.Fatal("managed connector backup was not exposed as rollback-capable")
	}
}

func TestInstallStableHostToIgnoresUIOnlyRebuildWithinSemanticVersion(t *testing.T) {
	base := t.TempDir()
	runtimeDir := filepath.Join(base, "runtime")
	source := filepath.Join(base, "Launcher.exe")
	if err := os.WriteFile(source, []byte("first-build"), 0o755); err != nil {
		t.Fatal(err)
	}
	first, changed, err := installStableHostTo(source, runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != first {
		t.Fatalf("unexpected first install changes: %#v", changed)
	}
	if err := os.WriteFile(source, []byte("second-build"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, secondChanged, err := installStableHostTo(source, runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(secondChanged) != 0 {
		t.Fatalf("same semantic Host version must remain stable: first=%q second=%q changed=%#v", first, second, secondChanged)
	}
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first-build" {
		t.Fatalf("UI-only rebuild overwrote semantic Host: %q", data)
	}
}

func TestManagedSkillUpdateBacksUpAndAdvancesManifest(t *testing.T) {
	machine := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machine)
	destination := filepath.Join(t.TempDir(), "brain2-connect", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	oldContent := []byte("old managed skill\n")
	if err := os.WriteFile(destination, oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSkillManifest("codex-desktop", destination, sha256Hex(oldContent)); err != nil {
		t.Fatal(err)
	}
	result, err := installSkillManaged("codex-desktop", destination, "new managed skill\n")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Conflict || len(result.ChangedPaths) != 2 {
		t.Fatalf("unexpected managed update result: %#v", result)
	}
	updated, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != "new managed skill\n" {
		t.Fatalf("skill was not updated: %q", updated)
	}
	backup, err := os.ReadFile(result.ChangedPaths[0])
	if err != nil || !bytes.Equal(backup, oldContent) {
		t.Fatalf("managed skill backup missing or incorrect: %q (err=%v)", backup, err)
	}
}

func TestLegacyManagedSkillHashesAreValidSHA256Values(t *testing.T) {
	for hash := range legacyManagedSkillHashes {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != sha256.Size {
			t.Fatalf("invalid managed legacy Skill hash %q", hash)
		}
	}
}

func TestManagedSkillUpdateStopsWhenUserModifiedInstalledSkill(t *testing.T) {
	machine := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machine)
	destination := filepath.Join(t.TempDir(), "brain2-connect", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	managedContent := []byte("managed skill\n")
	if err := os.WriteFile(destination, []byte("user edited skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSkillManifest("codex-desktop", destination, sha256Hex(managedContent)); err != nil {
		t.Fatal(err)
	}
	result, err := installSkillManaged("codex-desktop", destination, "next launcher skill\n")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Conflict || result.Updated || len(result.ChangedPaths) != 0 {
		t.Fatalf("user modification was not protected: %#v", result)
	}
	unchanged, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != "user edited skill\n" {
		t.Fatalf("conflicting skill was overwritten: %q", unchanged)
	}
}

func TestCurrentSkillBootstrapsManagedManifestWithoutRewrite(t *testing.T) {
	machine := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", machine)
	destination := filepath.Join(t.TempDir(), "brain2-connect", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("current skill\n")
	if err := os.WriteFile(destination, content, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := installSkillManaged("codex-desktop", destination, string(content))
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.Conflict || len(result.ChangedPaths) != 0 {
		t.Fatalf("current skill should only be adopted into management: %#v", result)
	}
	manifest, err := loadSkillManifest("codex-desktop")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ManagedSHA256 != sha256Hex(content) || manifest.Path != destination {
		t.Fatalf("unexpected skill manifest: %#v", manifest)
	}
}

func TestPrepareClaudeDesktopBundle(t *testing.T) {
	base := t.TempDir()
	hostDir := filepath.Join(base, "runtime")
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(hostDir, "brain2-mcp-host.exe")
	if err := os.WriteFile(host, []byte("fake-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	packagePath, err := prepareClaudeDesktopBundle(`D:\Brain2`, host)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	found := map[string]bool{}
	for _, file := range reader.File {
		found[file.Name] = true
	}
	if !found["manifest.json"] || !found["server/brain2-mcp-host.exe"] {
		t.Fatalf("incomplete MCPB contents: %#v", found)
	}
}

func TestDiagnoseMissingReceiptExplainsNetworkTimeout(t *testing.T) {
	message := diagnoseMissingReceipt("Codex CLI", []byte(`{"type":"error","message":"Reconnecting... 3/5 (request timed out)"}`))
	if !strings.Contains(message, "服务连接超时") || !strings.Contains(message, "MCP 已注册") {
		t.Fatalf("unexpected diagnostic: %s", message)
	}
}

func TestDiagnoseHandshakeFailureExplainsLogin(t *testing.T) {
	err := diagnoseHandshakeFailure("Claude Code", []byte("Login required"), errors.New("exit status 1"))
	if !strings.Contains(err.Error(), "尚未登录") {
		t.Fatalf("unexpected diagnostic: %v", err)
	}
}

func TestCodexVerificationUsesLeanEphemeralMCPOnlySession(t *testing.T) {
	var captured []string
	runner := func(_ context.Context, _ string, args []string, _ string) ([]byte, error) {
		captured = append([]string{}, args...)
		return []byte("ok"), nil
	}
	detection := Detection{ID: "codex", Name: "Codex CLI", Executable: "node.exe", PrefixArgs: []string{"codex.js"}}
	if _, err := runHandshake(context.Background(), runner, detection, `D:\Brain2`, "prompt"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(captured, " ")
	for _, required := range []string{"--ephemeral", "--sandbox read-only", "--json"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing %q in verification command: %s", required, joined)
		}
	}
	if strings.Contains(joined, "workspace-write") {
		t.Fatalf("verification must not request workspace-write sandbox: %s", joined)
	}
}

func TestParseWSLDistributionsDecodesWindowsUTF16Output(t *testing.T) {
	runes := utf16.Encode([]rune("\ufeffUbuntu\r\nUbuntu-Hermes\r\n"))
	data := make([]byte, len(runes)*2)
	for index, value := range runes {
		binary.LittleEndian.PutUint16(data[index*2:], value)
	}
	distros := parseWSLDistributions(data)
	if strings.Join(distros, ",") != "Ubuntu,Ubuntu-Hermes" {
		t.Fatalf("unexpected WSL distros: %#v", distros)
	}
}

func TestFindWSLHermesCommandPrefersDedicatedDistribution(t *testing.T) {
	runner := func(_ context.Context, _ string, args []string, _ string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if joined == "--list --quiet" {
			return []byte("Ubuntu\nUbuntu-Hermes\n"), nil
		}
		if strings.Contains(joined, "-d Ubuntu-Hermes") {
			return []byte("/usr/local/bin/hermes"), nil
		}
		return nil, errors.New("Hermes missing")
	}
	executable, prefix, distro := findWSLHermesCommand(context.Background(), `C:\Windows\System32\wsl.exe`, runner)
	if executable == "" || distro != "Ubuntu-Hermes" {
		t.Fatalf("unexpected detection: executable=%q distro=%q", executable, distro)
	}
	if strings.Join(prefix, " ") != "-d Ubuntu-Hermes -- /usr/local/bin/hermes" {
		t.Fatalf("unexpected WSL prefix: %#v", prefix)
	}
}

func TestRegisterMCPBuildsWSLHermesCommandWithLinuxPaths(t *testing.T) {
	calls := []string{}
	registered := false
	runner := func(_ context.Context, executable string, args []string, stdin string) ([]byte, error) {
		call := strings.Join(append([]string{executable}, args...), " ")
		calls = append(calls, call)
		if strings.HasSuffix(call, " mcp list") {
			if registered {
				return []byte("brain2-meow  all  enabled"), nil
			}
			return nil, errors.New("not registered")
		}
		if strings.Contains(call, " mcp add ") {
			if stdin != "y\n" {
				t.Fatalf("expected Hermes tool confirmation, got %q", stdin)
			}
			registered = true
		}
		return []byte("ok"), nil
	}
	detection := Detection{
		ID: "hermes", Name: "Hermes", Detected: true, Executable: `C:\Windows\System32\wsl.exe`,
		PrefixArgs: []string{"-d", "Ubuntu-Hermes", "--", "/usr/local/bin/hermes"},
	}
	err := registerMCP(context.Background(), runner, detection, `/mnt/d/Brain2`, `/home/hermes/.hermes/bin/brain2-mcp-host-test`, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	want := "wsl.exe -d Ubuntu-Hermes -- /usr/local/bin/hermes mcp add brain2-meow --command /home/hermes/.hermes/bin/brain2-mcp-host-test --args mcp-serve --registry /mnt/d/Brain2"
	if !strings.Contains(joined, want) {
		t.Fatalf("missing WSL Hermes registration %q in:\n%s", want, joined)
	}
}

func TestHermesVerificationUsesWSLPrefix(t *testing.T) {
	var captured []string
	runner := func(_ context.Context, _ string, args []string, _ string) ([]byte, error) {
		captured = append([]string{}, args...)
		return []byte("ok"), nil
	}
	detection := Detection{
		ID: "hermes", Name: "Hermes", Executable: "wsl.exe",
		PrefixArgs: []string{"-d", "Ubuntu-Hermes", "--", "/usr/local/bin/hermes"},
	}
	if _, err := runHandshake(context.Background(), runner, detection, `D:\Brain2`, "prompt"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "-d Ubuntu-Hermes -- /usr/local/bin/hermes chat -q prompt") {
		t.Fatalf("unexpected Hermes verification command: %s", joined)
	}
}

func TestWindowsPathToWSL(t *testing.T) {
	value, err := windowsPathToWSL(`D:\MeowLauncher\runtime\brain2-mcp-host.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if value != "/mnt/d/MeowLauncher/runtime/brain2-mcp-host.exe" {
		t.Fatalf("unexpected WSL path: %s", value)
	}
}
