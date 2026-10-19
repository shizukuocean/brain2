//go:build !launcher

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"meow-launcher/internal/brain2"
	"meow-launcher/internal/settings"
)

func TestWindowsBuildsEmbedTheSuppliedProductIcons(t *testing.T) {
	pngBytes, err := os.ReadFile(filepath.Join("assets", "brain2-manager-icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(pngBytes)); got != "5d271ec06b1889dab9b65e984cdadb81881d69a2c44db836513c7dbda77c52e9" {
		t.Fatalf("Manager icon source changed: %s", got)
	}
	icoBytes, err := os.ReadFile(filepath.Join("build", "windows", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	if len(icoBytes) < 6 || binary.LittleEndian.Uint16(icoBytes[2:4]) != 1 || binary.LittleEndian.Uint16(icoBytes[4:6]) != 9 {
		t.Fatal("Manager Wails icon is not a nine-size Windows icon")
	}
	launcherPNGBytes, err := os.ReadFile(filepath.Join("assets", "meow-launcher-icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(launcherPNGBytes)); got != "1d7aec17360ddf8d05fdcf991b9205c3ca1e33f9d55bc3f25e7480eb0065775d" {
		t.Fatalf("Launcher icon source changed: %s", got)
	}
	for _, scriptPath := range []string{"build-split-windows.ps1", filepath.Join("scripts", "build-update-preview.ps1")} {
		scriptBytes, readErr := os.ReadFile(scriptPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		script := string(scriptBytes)
		for _, required := range []string{
			`assets\brain2-manager-icon.png`,
			`manager_icon_windows_amd64.syso`,
			`Manager icon resource generation failed.`,
			`Remove-Item -LiteralPath $managerResource`,
			`assets\meow-launcher-icon.png`,
			`launcher_icon_windows_amd64.syso`,
			`Launcher icon resource generation failed.`,
			`Remove-Item -LiteralPath $launcherResource`,
		} {
			if !strings.Contains(script, required) {
				t.Fatalf("%s is missing Manager icon build behavior %q", scriptPath, required)
			}
		}
	}
}

func TestManagerFrontendMakesApplicationUpdatesDiscoverableAndHandsThemToLauncher(t *testing.T) {
	sourceBytes, err := os.ReadFile(filepath.Join("frontend", "src", "App.vue"))
	if err != nil {
		t.Fatal(err)
	}
	styleBytes, err := os.ReadFile(filepath.Join("frontend", "src", "style.css"))
	if err != nil {
		t.Fatal(err)
	}
	source, style := string(sourceBytes), string(styleBytes)
	for _, required := range []string{
		`await checkApplicationUpdate()`,
		`settingsUpdateSummaryLabel`,
		`await backend.detectAgents()`,
		`更新连接组件`,
		`applicationUpdate.status === 'update_available'`,
		`'has-notification': applicationUpdate.status === 'update_available'`,
		`class="nav-notification-dot"`,
		`Settings, update available`,
		`openingLauncherUpdate.value = true`,
		`await backend.openApplicationUpdate()`,
		`Opening Launcher…`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("Manager frontend is missing application update behavior %q", required)
		}
	}
	if count := strings.Count(source, `<section class="settings-update"`); count != 1 {
		t.Fatalf("Manager settings must expose one unified update surface, got %d", count)
	}
	if strings.Contains(source, `Brain2 Manager 应用更新`) {
		t.Fatal("Manager settings still exposes a duplicate application-update section")
	}
	if !strings.Contains(style, `.side-nav .nav-notification-dot`) || !strings.Contains(style, `flex: 0 0 8px`) {
		t.Fatal("Manager settings update dot is missing its visual style")
	}
}

func TestReleaseStagingCanRequireRealInPlaceUpgradeEvidence(t *testing.T) {
	stageBytes, err := os.ReadFile(filepath.Join("scripts", "stage-update-release.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	updateBytes, err := os.ReadFile(filepath.Join("scripts", "test-update-in-place.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	stage, update := string(stageBytes), string(updateBytes)
	for _, required := range []string{
		`[string]$PreviousManagerInstaller = ''`,
		`[string]$PreviousHelperInstaller = ''`,
		`[string]$PreviousLauncherInstaller = ''`,
		`PreviousManagerInstaller, PreviousHelperInstaller and PreviousLauncherInstaller must be supplied together.`,
		`scripts\test-update-in-place.ps1`,
		`throw 'In-place update validation failed.'`,
	} {
		if !strings.Contains(stage, required) {
			t.Fatalf("release staging is missing in-place update gate %q", required)
		}
	}
	for _, required := range []string{
		`function Get-InstallerVersion {`,
		`$previousManagerVersion = Get-InstallerVersion $PreviousManagerInstaller 'brain2-manager'`,
		`$previousHelperVersion = Get-InstallerVersion $PreviousHelperInstaller 'dev-helper'`,
		`$previousLauncherVersion = Get-InstallerVersion $PreviousLauncherInstaller 'meow-launcher'`,
		`Invoke-Installer $PreviousManagerInstaller 'brain2-manager' $previousManagerVersion $managerTarget $managerOldReceipt`,
		`Invoke-Installer $managerCurrent.Path 'brain2-manager' $managerCurrent.Release.version $managerTarget $managerNewReceipt`,
		`Invoke-Installer $PreviousHelperInstaller 'dev-helper' $previousHelperVersion $helperTarget $helperOldReceipt`,
		`Invoke-Installer $helperCurrent.Path 'dev-helper' $helperCurrent.Release.version $helperTarget $helperNewReceipt`,
		`'preserve-manager'`,
		`'preserve-helper'`,
		`'preserve-launcher'`,
		`throw 'Manager executable was not replaced.'`,
		`throw 'Helper executable was not replaced.'`,
		`update_manifest_url`,
		`MEOW_DESKTOP_SHORTCUT_TEST_ROOT`,
		`Brain2 Manager.lnk`,
		`Dev Helper.lnk`,
		`Meow Launcher.lnk`,
		`desktop shortcut does not point to the updated executable`,
		`legacy Launcher started the one-shot installer`,
		`old_owned_files_removed = $true`,
		`Launcher migration removed an unowned user file.`,
	} {
		if !strings.Contains(update, required) {
			t.Fatalf("in-place update gate is missing evidence %q", required)
		}
	}
}

func TestReleaseBuildPackagesTheWailsHelperPayload(t *testing.T) {
	buildBytes, err := os.ReadFile(filepath.Join("scripts", "build-update-preview.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	testBytes, err := os.ReadFile(filepath.Join("scripts", "test-update-preview.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	build, testScript := string(buildBytes), string(testBytes)
	for _, required := range []string{
		`npm.cmd run desktop:wails:package`,
		`dist-wails`,
		`runtime\helper-backend.exe`,
		`resources\build-info.json`,
	} {
		if !strings.Contains(build, required) {
			t.Fatalf("release build is missing Wails Helper payload behavior %q", required)
		}
		if required != `npm.cmd run desktop:wails:package` && required != `dist-wails` && !strings.Contains(testScript, required) {
			t.Fatalf("release E2E is missing Wails Helper evidence %q", required)
		}
	}
	if strings.Contains(testScript, `resources\app.asar`) {
		t.Fatal("release E2E still requires the retired Electron app.asar payload")
	}
	for _, required := range []string{
		`MEOW_DESKTOP_SHORTCUT_TEST_ROOT`,
		`Brain2 Manager.lnk`,
		`Dev Helper.lnk`,
		`Meow Launcher.lnk`,
		`Launcher self-update did not refresh the desktop shortcut target.`,
	} {
		if !strings.Contains(testScript, required) {
			t.Fatalf("release E2E is missing desktop shortcut evidence %q", required)
		}
	}
}

func TestFirstRunCanAttachOrdinaryObsidianVaultWithoutDeveloperPaths(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", filepath.Join(temp, "portable-machine-root"))
	t.Setenv("MEOW_LAUNCHER_SIMULATE_NO_OBSIDIAN", "1")
	app := &App{
		statePath:    filepath.Join(temp, "fresh-user", "state.json"),
		registryPath: filepath.Join(temp, "portable-machine-root", "runtime", "brain2-registry.json"),
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Workspace || state.ObsidianReady || state.Brain2Root != "" {
		t.Fatalf("fresh user was not shown a clean first-run state: %#v", state)
	}
	vault := filepath.Join(temp, "User Vault")
	if err := os.MkdirAll(filepath.Join(vault, ".obsidian"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(vault, "existing.md")
	if err := os.WriteFile(note, []byte("user-owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err = app.AttachBrain2(vault)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Workspace || state.Brain2Root != filepath.Clean(vault) {
		t.Fatalf("ordinary Obsidian vault did not become usable: %#v", state)
	}
	if data, err := os.ReadFile(note); err != nil || string(data) != "user-owned\n" {
		t.Fatalf("Launcher changed existing user content: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(app.registryPath); err != nil {
		t.Fatalf("portable machine registry missing: %v", err)
	}
	report, err := app.RunHealthCheck()
	if err != nil || report.OverallStatus == "critical" {
		t.Fatalf("attached ordinary vault was not operational: report=%#v err=%v", report, err)
	}
}

func TestManagerReadsSharedApplicationUpdateManifest(t *testing.T) {
	manifestPath, err := filepath.Abs(filepath.Join("testdata", "update", "manifest-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_UPDATE_MANIFEST", manifestPath)
	result := NewApp().CheckApplicationUpdate()
	if result.Status != "update_available" || result.Release == nil || result.Release.ProductID != "brain2-manager" {
		t.Fatalf("unexpected Manager update result %#v", result)
	}
}

func TestAppBrain2Lifecycle(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("MEOW_LAUNCHER_MACHINE_ROOT", filepath.Join(temp, "machine-root"))
	app := &App{statePath: filepath.Join(temp, "launcher-state.json"), registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	root := filepath.Join(temp, "Brain2")

	state, err := app.InstallBrain2(root)
	if err != nil {
		t.Fatalf("InstallBrain2 returned error: %v", err)
	}
	if !state.Workspace || state.Brain2Root != filepath.Clean(root) {
		t.Fatalf("unexpected installed state: %#v", state)
	}
	registryPath := app.registryPath
	if _, err := os.Stat(registryPath); err != nil {
		t.Fatalf("shared product registry missing: %v", err)
	}

	state, err = app.CreateInternalProject("视觉壳验收", "验证 Wails 绑定")
	if err != nil {
		t.Fatalf("CreateInternalProject returned error: %v", err)
	}
	if state.ProjectCount != 1 {
		t.Fatalf("expected one project, got %#v", state)
	}

	external := filepath.Join(temp, "external-source")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(external, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err = app.AttachExternalProject("外部项目", external, "不移动源目录")
	if err != nil {
		t.Fatalf("AttachExternalProject returned error: %v", err)
	}
	if state.ProjectCount != 2 {
		t.Fatalf("expected two projects, got %#v", state)
	}
	archiveTask, err := app.PrepareProjectArchivePrompt("codex", state.Projects[0].ID)
	if err != nil {
		t.Fatalf("PrepareProjectArchivePrompt returned error: %v", err)
	}
	if !strings.Contains(archiveTask.Prompt, "项目档案草稿") || !strings.Contains(archiveTask.Prompt, "未获明确授权不得写入") {
		t.Fatalf("project archive task did not preserve the Agent/user boundary: %s", archiveTask.Prompt)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("external source changed: data=%q err=%v", data, err)
	}

	connection, err := app.PrepareAgentPrompt("codex", "brain", "", "", "")
	if err != nil {
		t.Fatalf("PrepareAgentPrompt returned error: %v", err)
	}
	state, err = app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.RuntimeReady || state.AgentCount != 1 {
		t.Fatalf("prepared connection must not be reported as ready: %#v", state)
	}
	task, err := brain2.LoadConnectionTask(root, connection.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := brain2.CompleteConnection(root, brain2.HandshakeInput{
		ConnectionID: task.ConnectionID, Nonce: task.Nonce, AgentID: "codex", AgentVersion: "test",
		Surface: "cli", ProtocolVersion: brain2.ConnectionProtocolVersion,
		ReadFiles: []string{"NAV.md"}, HealthStatus: "healthy",
	}); err != nil {
		t.Fatal(err)
	}
	state, err = app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if !state.RuntimeReady {
		t.Fatalf("validated receipt not reflected in state: %#v", state)
	}

	health, err := app.RunHealthCheck()
	if err != nil {
		t.Fatalf("RunHealthCheck returned error: %v", err)
	}
	if health.OverallStatus != "healthy" || health.Score != 100 {
		t.Fatalf("unexpected health report: %#v", health)
	}
}

func TestPrepareObsidianInstallPromptKeepsAgentAndUserBoundaries(t *testing.T) {
	app := &App{}
	target := filepath.Join(t.TempDir(), "Obsidian")
	prompt, err := app.PrepareObsidianInstallPrompt(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		target,
		"https://obsidian.md/download",
		"https://github.com/obsidianmd/obsidian-releases/releases/latest",
		"不要静默安装到其他目录",
		"不修改我的 Obsidian Vault",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("Obsidian install prompt missing %q:\n%s", required, prompt)
		}
	}
}

func TestPrepareProjectScanCreatesBoundedAgentTask(t *testing.T) {
	temp := t.TempDir()
	app := &App{statePath: filepath.Join(temp, "launcher-state.json"), registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	brainRoot := filepath.Join(temp, "Brain2")
	if _, err := app.InstallBrain2(brainRoot); err != nil {
		t.Fatal(err)
	}
	scanRoot := filepath.Join(temp, "projects")
	if err := os.MkdirAll(scanRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := app.PrepareProjectScan("codex", scanRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{filepath.Clean(scanRoot), "brain2_submit_project_candidates", "不得直接修改 .brain2/projects.json", "Brain2 工作区", "shared_repository_project_id"} {
		if !strings.Contains(result.Prompt, required) {
			t.Fatalf("scan prompt missing %q: %s", required, result.Prompt)
		}
	}
	if result.TaskID == "" {
		t.Fatal("scan task id was not returned")
	}
}

func TestBrain2FirstCandidateCreatesDedicatedArchivePromptWithoutEarlyRegistration(t *testing.T) {
	temp := t.TempDir()
	app := &App{statePath: filepath.Join(temp, "launcher-state.json"), registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	brainRoot := filepath.Join(temp, "Brain2")
	if _, err := app.InstallBrain2(brainRoot); err != nil {
		t.Fatal(err)
	}
	discovery, err := app.PrepareProjectScan("codex", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"brain2_first", "不得为了找项目而全盘遍历磁盘", "code_project、application、product", "不等于 Brain2 Manager 产品"} {
		if !strings.Contains(discovery.Prompt, required) {
			t.Fatalf("Brain2-first prompt missing %q: %s", required, discovery.Prompt)
		}
	}
	sourceRoot := filepath.Join(temp, "Launcher")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	task, err := brain2.LoadProjectScanTask(brainRoot, discovery.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := brain2.SubmitProjectCandidates(brainRoot, task, []brain2.ProjectCandidateInput{{
		Name: "Launcher", RootPath: sourceRoot, Confidence: 1,
		Evidence: []string{"Brain2 交接指向该目录"}, DetectedKind: "product",
	}})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := app.PrepareProjectCandidateArchive("codex", candidates.Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"brain2_submit_project_archive", "archive_task_id", filepath.Clean(sourceRoot), "无需让用户确认技术字段"} {
		if !strings.Contains(archive.Prompt, required) {
			t.Fatalf("archive prompt missing %q: %s", required, archive.Prompt)
		}
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.ProjectCount != 0 || len(state.ProjectCandidates) != 1 || state.ProjectCandidates[0].Status != "selected_for_archiving" {
		t.Fatalf("candidate was registered before Agent receipt: %#v", state)
	}
}

func TestBatchProjectCandidateArchiveCreatesOnePromptWithIndependentTasks(t *testing.T) {
	temp := t.TempDir()
	app := &App{statePath: filepath.Join(temp, "launcher-state.json"), registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	brainRoot := filepath.Join(temp, "Brain2")
	if _, err := app.InstallBrain2(brainRoot); err != nil {
		t.Fatal(err)
	}
	discovery, err := app.PrepareProjectScan("codex", "")
	if err != nil {
		t.Fatal(err)
	}
	firstRoot := filepath.Join(temp, "Launcher")
	secondRoot := filepath.Join(temp, "Protector")
	for _, projectRoot := range []string{firstRoot, secondRoot} {
		if err := os.MkdirAll(projectRoot, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scanTask, err := brain2.LoadProjectScanTask(brainRoot, discovery.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := brain2.SubmitProjectCandidates(brainRoot, scanTask, []brain2.ProjectCandidateInput{
		{Name: "Launcher", RootPath: firstRoot, Confidence: 1, SuggestedGoal: "统一启动入口", Evidence: []string{"Brain2 交接指向该目录"}, DetectedKind: "product"},
		{Name: "Protector", RootPath: secondRoot, Confidence: 0.9, SuggestedGoal: "安全检查应用", Evidence: []string{"发现应用入口"}, DetectedKind: "application"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.PrepareProjectCandidateArchives("codex", []string{candidates.Candidates[0].ID, candidates.Candidates[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0].TaskID == result.Items[1].TaskID {
		t.Fatalf("unexpected batch result: %#v", result)
	}
	for _, required := range []string{"Launcher", "Protector", firstRoot, secondRoot, result.Items[0].TaskID, result.Items[1].TaskID, "逐项目分别调用 brain2_submit_project_archive", "不得只提交一次代表整批"} {
		if !strings.Contains(result.Prompt, required) {
			t.Fatalf("batch prompt missing %q: %s", required, result.Prompt)
		}
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.ProjectCount != 0 || len(state.ProjectCandidates) != 2 {
		t.Fatalf("batch archive registered projects too early: %#v", state)
	}
	for _, candidate := range state.ProjectCandidates {
		if candidate.Status != "selected_for_archiving" {
			t.Fatalf("candidate was not selected for archiving: %#v", state.ProjectCandidates)
		}
	}
}

func TestAppRejectsActionsWithoutWorkspace(t *testing.T) {
	temp := t.TempDir()
	app := &App{statePath: filepath.Join(temp, "missing-state.json"), registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	if _, err := app.CreateInternalProject("x", ""); err == nil {
		t.Fatal("expected project creation without Brain2 to fail")
	}
	if _, err := app.PrepareCodexRuntime(); err == nil {
		t.Fatal("expected runtime preparation without Brain2 to fail")
	}
}

func TestDeviceSwitchKeepsRemoteAgentStateReadOnly(t *testing.T) {
	temp := t.TempDir()
	app := &App{statePath: filepath.Join(temp, "launcher-state.json"), registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	root := filepath.Join(temp, "Brain2")
	if _, err := app.InstallBrain2(root); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddDevice("B"); err != nil {
		t.Fatal(err)
	}
	if err := brain2.MarkAgentRegisteredForDevice(root, "B", "codex", "quick"); err != nil {
		t.Fatal(err)
	}
	state, err := app.SwitchDevice("A")
	if err != nil {
		t.Fatal(err)
	}
	if !state.SelectedDeviceLocal || state.AgentCount != 0 {
		t.Fatalf("A inherited B state: %#v", state)
	}
	state, err = app.SwitchDevice("B")
	if err != nil {
		t.Fatal(err)
	}
	if state.SelectedDeviceLocal || state.AgentCount != 1 || state.Agents[0].Name != "Codex CLI (B)" {
		t.Fatalf("B remote state was not shown correctly: %#v", state)
	}
	if _, err := app.PrepareAgentPrompt("codex", "brain", "", "", ""); err == nil || !strings.Contains(err.Error(), "远程记录") {
		t.Fatalf("remote Agent action was not blocked: %v", err)
	}
	state, err = app.RemoveDevice("B")
	if err != nil {
		t.Fatal(err)
	}
	if state.SelectedDeviceID != "A" || len(state.Devices) != 1 || state.Devices[0].ID != "A" {
		t.Fatalf("removing B did not return to local A: %#v", state)
	}
	if _, err := app.RemoveDevice("A"); err == nil {
		t.Fatal("removing the local device should fail")
	}
	state, err = app.AddDevice("B")
	if err != nil {
		t.Fatal(err)
	}
	state, err = app.SwitchDevice("B")
	if err != nil || state.AgentCount != 1 {
		t.Fatalf("B history did not return after re-add: %#v err=%v", state, err)
	}
}

func TestEnglishAgentDiagnosticsAreLocalized(t *testing.T) {
	message := localizedAgentError("health needs_attention; governed archive completed without claiming a successful handshake")
	if strings.Contains(message, "needs_attention") || !strings.Contains(message, "握手未成功") {
		t.Fatalf("diagnostic was not localized: %q", message)
	}
}

func TestAmbiguousMachineRequiresExplicitDeviceChoice(t *testing.T) {
	temp := t.TempDir()
	statePath := filepath.Join(temp, "launcher-state.json")
	app := &App{statePath: statePath, registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	root := filepath.Join(temp, "Brain2")
	if err := brain2.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := settings.SaveTo(statePath, root); err != nil {
		t.Fatal(err)
	}
	devices := `{"schema_version":"0.1","devices":[{"id":"A","name":"设备 A","brain2_path":"C:\\Other\\Brain2","machine_name":"OTHER-PC","status":"registered"}]}`
	if err := os.WriteFile(filepath.Join(root, ".brain2", "devices.json"), []byte(devices), 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := app.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if !state.DeviceSetupRequired || state.LocalDeviceID != "" || state.SelectedDeviceID != "" {
		t.Fatalf("ambiguous machine was silently assigned a device: %#v", state)
	}
	if _, err := app.PrepareAgentPrompt("codex", "brain", "", "", ""); err == nil || !strings.Contains(err.Error(), "确认这台机器") {
		t.Fatalf("local action was allowed before device choice: %v", err)
	}
	state, err = app.SetLocalDevice("B")
	if err != nil {
		t.Fatal(err)
	}
	if state.DeviceSetupRequired || state.LocalDeviceID != "B" || state.SelectedDeviceID != "B" {
		t.Fatalf("explicit B choice did not register the local machine: %#v", state)
	}
}

func TestConnectorUpdatePreferenceRemainsUserControlled(t *testing.T) {
	temp := t.TempDir()
	statePath := filepath.Join(temp, "launcher-state.json")
	app := &App{statePath: statePath, registryPath: filepath.Join(temp, "machine", "brain2-registry.json")}
	if _, err := app.InstallBrain2(filepath.Join(temp, "Brain2")); err != nil {
		t.Fatal(err)
	}
	if err := app.IgnoreConnectorUpdate("codex"); err != nil {
		t.Fatal(err)
	}
	persisted, err := settings.LoadFrom(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.IgnoredConnectorVersions["codex"] != brain2.MCPHostVersion {
		t.Fatalf("ignored version was not persisted: %#v", persisted.IgnoredConnectorVersions)
	}
	if err := app.RestoreConnectorUpdate("codex"); err != nil {
		t.Fatal(err)
	}
	persisted, err = settings.LoadFrom(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := persisted.IgnoredConnectorVersions["codex"]; ok {
		t.Fatalf("restored reminder still marked ignored: %#v", persisted.IgnoredConnectorVersions)
	}
}

func TestLauncherProfileArgs(t *testing.T) {
	root, profile, err := launcherProfileFromArgs([]string{"--data-root", `D:\MeowLauncher\profiles\产品测试`, "--profile", "产品测试"})
	if err != nil {
		t.Fatal(err)
	}
	if root != `D:\MeowLauncher\profiles\产品测试` || profile != "产品测试" {
		t.Fatalf("unexpected profile args: root=%q profile=%q", root, profile)
	}
	if _, _, err := launcherProfileFromArgs([]string{"--data-root", "relative"}); err == nil {
		t.Fatal("expected relative data root to fail")
	}
}

func TestObsidianDetectionCanBeSimulatedForProductTesting(t *testing.T) {
	t.Setenv("MEOW_LAUNCHER_SIMULATE_NO_OBSIDIAN", "1")
	if got := detectObsidian(); got != "" {
		t.Fatalf("simulated missing Obsidian returned %q", got)
	}
	if !hasLauncherFlag([]string{"--profile", "产品测试", "--simulate-no-obsidian"}, "--simulate-no-obsidian") {
		t.Fatal("product-test flag was not detected")
	}
}

func TestDetectObsidianPersistsAutoDetectedPath(t *testing.T) {
	temp := t.TempDir()
	executable := filepath.Join(temp, "Obsidian.exe")
	if err := os.WriteFile(executable, []byte("test executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEOW_OBSIDIAN_PATH", executable)
	app := &App{statePath: filepath.Join(temp, "state.json"), registryPath: filepath.Join(temp, "registry.json")}

	state, err := app.DetectObsidian()
	if err != nil {
		t.Fatal(err)
	}
	if !state.ObsidianReady || state.ObsidianPath != filepath.Clean(executable) {
		t.Fatalf("explicit detection did not return the discovered executable: %#v", state)
	}
	persisted, err := app.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ObsidianPath != filepath.Clean(executable) {
		t.Fatalf("detected path was not persisted: %#v", persisted)
	}
}

func TestObsidianAcceptanceBuildUsesIsolatedProfile(t *testing.T) {
	root, profile := launcherAcceptanceProfile("", "", `C:\Users\test\AppData\Local\MeowLauncher`)
	if root != `C:\Users\test\AppData\Local\MeowLauncher\acceptance-ob-install` {
		t.Fatalf("unexpected acceptance data root: %q", root)
	}
	if profile != "OB 安装验收" {
		t.Fatalf("unexpected acceptance profile: %q", profile)
	}

	customRoot, customProfile := launcherAcceptanceProfile(`D:\isolated`, "自定义验收", `C:\ignored`)
	if customRoot != `D:\isolated` || customProfile != "自定义验收" {
		t.Fatalf("explicit acceptance profile was overwritten: root=%q profile=%q", customRoot, customProfile)
	}
}
