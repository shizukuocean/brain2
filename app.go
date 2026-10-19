package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"meow-launcher/internal/agentconnect"
	"meow-launcher/internal/brain2"
	"meow-launcher/internal/buildinfo"
	"meow-launcher/internal/lifecycle"
	"meow-launcher/internal/productlaunch"
	"meow-launcher/internal/productregistry"
	"meow-launcher/internal/settings"
	"meow-launcher/internal/updatecontract"
)

//go:embed packages/skills/brain2-connect/SKILL.md
var brain2ConnectSkill string

type ProjectSummary struct {
	ID                        string `json:"id"`
	Name                      string `json:"name"`
	Mode                      string `json:"mode"`
	Status                    string `json:"status"`
	SourcePath                string `json:"source_path"`
	ManifestPath              string `json:"manifest_path"`
	AssetStatus               string `json:"asset_status"`
	PathStatus                string `json:"path_status"`
	TruthStatus               string `json:"truth_status"`
	SharedRepositoryProjectID string `json:"shared_repository_project_id,omitempty"`
	MachineRecord             bool   `json:"machine_record"`
	ProtectorEligible         bool   `json:"protector_eligible"`
	HelperEligible            bool   `json:"helper_eligible"`
}

type LauncherState struct {
	Brain2Root          string                    `json:"brain2_root"`
	Workspace           bool                      `json:"workspace"`
	ProjectCount        int                       `json:"project_count"`
	AgentCount          int                       `json:"agent_count"`
	RuntimeReady        bool                      `json:"runtime_ready"`
	HealthStatus        string                    `json:"health_status"`
	HealthScore         int                       `json:"health_score"`
	Projects            []ProjectSummary          `json:"projects"`
	Agents              []AgentSummary            `json:"agents"`
	LauncherBuild       string                    `json:"launcher_build"`
	LauncherProfile     string                    `json:"launcher_profile"`
	ObsidianReady       bool                      `json:"obsidian_ready"`
	ObsidianPath        string                    `json:"obsidian_path"`
	AcceptanceMode      string                    `json:"acceptance_mode"`
	ProjectCandidates   []brain2.ProjectCandidate `json:"project_candidates"`
	ProtectorAvailable  bool                      `json:"protector_available"`
	HelperAvailable     bool                      `json:"helper_available"`
	LocalDeviceID       string                    `json:"local_device_id"`
	SelectedDeviceID    string                    `json:"selected_device_id"`
	SelectedDeviceLocal bool                      `json:"selected_device_local"`
	DeviceSetupRequired bool                      `json:"device_setup_required"`
	LocalDeviceSource   string                    `json:"local_device_source"`
	Devices             []DeviceSummary           `json:"devices"`
}

type DeviceSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Brain2Path  string `json:"brain2_path"`
	MachineName string `json:"machine_name"`
	Local       bool   `json:"local"`
	Status      string `json:"status"`
}

type ProjectScanResult struct {
	TaskID      string `json:"task_id"`
	Message     string `json:"message"`
	Prompt      string `json:"prompt"`
	Brain2Root  string `json:"brain2_root"`
	WorkspaceID string `json:"workspace_id"`
	ScanRoot    string `json:"scan_root"`
}

type ProjectArchiveResult struct {
	TaskID      string `json:"task_id"`
	Message     string `json:"message"`
	Prompt      string `json:"prompt"`
	CandidateID string `json:"candidate_id"`
	ProjectName string `json:"project_name"`
}

type ProjectArchiveBatchItem struct {
	TaskID      string `json:"task_id"`
	CandidateID string `json:"candidate_id"`
	ProjectName string `json:"project_name"`
}

type ProjectArchiveBatchResult struct {
	Message string                    `json:"message"`
	Prompt  string                    `json:"prompt"`
	Items   []ProjectArchiveBatchItem `json:"items"`
}

type ProtectorCatalogLaunchResult struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	CatalogID      string `json:"catalog_id"`
	ProjectCount   int    `json:"project_count"`
	ProductVersion string `json:"product_version"`
	ExpiresAt      string `json:"expires_at"`
}

type HelperLaunchResult struct {
	Status         string   `json:"status"`
	Message        string   `json:"message"`
	TaskID         string   `json:"task_id"`
	ProjectID      string   `json:"project_id"`
	ProductVersion string   `json:"product_version"`
	AcceptedScopes []string `json:"accepted_scopes"`
	ExpiresAt      string   `json:"expires_at"`
}

type AgentSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Surface     string `json:"surface"`
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	ConnectedAt string `json:"connected_at"`
	LastError   string `json:"last_error"`
}

type App struct {
	ctx          context.Context
	statePath    string
	registryPath string
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) CheckApplicationUpdate() updatecontract.CheckResult {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	source := strings.TrimSpace(os.Getenv("MEOW_UPDATE_MANIFEST"))
	if source == "" {
		source = buildinfo.UpdateManifestURL
	}
	return updatecontract.Check(ctx, source, "brain2-manager", buildinfo.ManagerVersion, "preview")
}

func (a *App) OpenApplicationUpdate() error {
	return lifecycle.OpenLauncherUpdate("brain2-manager", "restart")
}

func (a *App) GetState() (LauncherState, error) {
	state := LauncherState{
		HealthStatus:       "not_checked",
		Projects:           []ProjectSummary{},
		Agents:             []AgentSummary{},
		ProjectCandidates:  []brain2.ProjectCandidate{},
		LauncherBuild:      "v0.6.6 WIP",
		LauncherProfile:    strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_PROFILE")),
		AcceptanceMode:     strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_ACCEPTANCE_MODE")),
		ProtectorAvailable: productlaunch.Available(strings.TrimSpace(os.Getenv("MEOW_PROTECTOR_DEV_ROOT"))),
		HelperAvailable:    productlaunch.HelperAvailable(strings.TrimSpace(os.Getenv("MEOW_DEV_HELPER_ROOT"))),
		Devices:            []DeviceSummary{},
	}
	persisted, err := a.loadSettings()
	if err != nil {
		return state, err
	}
	state.ObsidianPath = detectObsidian(persisted.ObsidianPath)
	state.ObsidianReady = state.ObsidianPath != ""
	state.Brain2Root = persisted.Brain2Root
	if !brain2.IsWorkspace(state.Brain2Root) {
		return state, nil
	}
	state.Workspace = true
	settingsChanged := false
	deviceRegistry, err := brain2.LoadDevicesOrEmpty(state.Brain2Root)
	if err != nil {
		return state, err
	}
	hostname, _ := os.Hostname()
	resolution := brain2.ResolveLocalDeviceID(deviceRegistry, state.Brain2Root, hostname, persisted.LocalDeviceID)
	state.LocalDeviceSource = resolution.Source
	if resolution.DeviceID == "" {
		state.DeviceSetupRequired = true
		if persisted.LocalDeviceID != "" || persisted.SelectedDeviceID != "" {
			settingsChanged = true
		}
		persisted.LocalDeviceID = ""
		persisted.SelectedDeviceID = ""
	} else {
		if persisted.LocalDeviceID != resolution.DeviceID {
			persisted.LocalDeviceID = resolution.DeviceID
			persisted.SelectedDeviceID = resolution.DeviceID
			settingsChanged = true
		}
		deviceRegistry, err = brain2.EnsureDeviceRegistry(state.Brain2Root, persisted.LocalDeviceID)
		if err != nil {
			return state, err
		}
		if strings.TrimSpace(persisted.SelectedDeviceID) == "" {
			persisted.SelectedDeviceID = persisted.LocalDeviceID
			settingsChanged = true
		}
		if _, exists := brain2.DeviceByID(deviceRegistry, persisted.SelectedDeviceID); !exists {
			persisted.SelectedDeviceID = persisted.LocalDeviceID
			settingsChanged = true
		}
	}
	if settingsChanged {
		if err := a.saveSettingsState(persisted); err != nil {
			return state, err
		}
	}
	state.LocalDeviceID = persisted.LocalDeviceID
	state.SelectedDeviceID = persisted.SelectedDeviceID
	state.SelectedDeviceLocal = state.LocalDeviceID != "" && state.LocalDeviceID == state.SelectedDeviceID
	for _, device := range deviceRegistry.Devices {
		state.Devices = append(state.Devices, DeviceSummary{ID: device.ID, Name: device.Name, Brain2Path: device.Brain2Path, MachineName: device.MachineName, Local: device.ID == state.LocalDeviceID, Status: device.Status})
	}
	if err := a.syncProductRegistry(state.Brain2Root); err != nil {
		return state, err
	}
	projects, err := brain2.LoadProjects(state.Brain2Root)
	if err != nil {
		return state, err
	}
	selectedMachineName := ""
	if selectedDevice, exists := brain2.DeviceByID(deviceRegistry, state.SelectedDeviceID); exists {
		selectedMachineName = selectedDevice.MachineName
	}
	for _, project := range projects.Projects {
		summary := ProjectSummary{
			ID: project.ID, Name: project.Name, Mode: project.Mode, Status: project.Status,
			ManifestPath: project.ManifestPath, AssetStatus: project.AssetStatus,
			PathStatus: project.PathStatus, TruthStatus: project.TruthStatus,
			SharedRepositoryProjectID: project.SharedRepositoryProjectID,
		}
		for _, capability := range project.Capabilities {
			if capability == "machine_project_record" {
				summary.MachineRecord = true
				break
			}
		}
		if selectedPath, available := productlaunch.ProjectPathOnDevice(project, state.SelectedDeviceID, selectedMachineName); available {
			summary.SourcePath = selectedPath
		} else if project.SourcePath != nil {
			summary.SourcePath = *project.SourcePath
		}
		summary.ProtectorEligible = state.SelectedDeviceLocal && productlaunch.ProjectAvailableOnDevice(project, state.LocalDeviceID, hostname)
		summary.HelperEligible = summary.ProtectorEligible
		state.Projects = append(state.Projects, summary)
	}
	state.ProjectCount = len(state.Projects)
	candidates, err := brain2.LoadProjectCandidates(state.Brain2Root)
	if err != nil {
		return state, err
	}
	state.ProjectCandidates = candidates.Candidates
	if state.SelectedDeviceID == "" {
		return state, nil
	}
	agents, err := brain2.LoadAgentsForDevice(state.Brain2Root, state.SelectedDeviceID)
	if err != nil {
		return state, err
	}
	state.AgentCount = len(agents.Agents)
	for _, agent := range agents.Agents {
		lastError := localizedAgentError(agent.LastError)
		if agent.ConnectionStatus == string(brain2.ConnectionRegistered) || agent.ConnectionStatus == string(brain2.ConnectionConnected) {
			// v0.5.7 stored optional online-verification failures beside an otherwise
			// valid local install. That check is no longer part of the primary flow.
			lastError = ""
		}
		state.Agents = append(state.Agents, AgentSummary{
			ID: agent.ID, Name: fmt.Sprintf("%s (%s)", agent.Name, state.SelectedDeviceID), Surface: agent.Surface,
			Status: agent.ConnectionStatus, Mode: agent.ConnectionMode,
			ConnectedAt: agent.ConnectedAt, LastError: lastError,
		})
		if agent.ConnectionStatus == string(brain2.ConnectionRegistered) || agent.ConnectionStatus == string(brain2.ConnectionConnected) {
			state.RuntimeReady = true
		}
	}
	return state, nil
}

func localizedAgentError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "health needs_attention") && strings.Contains(lower, "without claiming a successful handshake"):
		return "健康检查有需要处理的项目；受控归档已完成，但本次 Agent 握手未成功。"
	case strings.Contains(lower, "network timeout") || strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		return "网络连接超时，请稍后重试。"
	case strings.Contains(lower, "connection refused"):
		return "连接被拒绝，请确认 Agent 已启动并重新接入。"
	}
	for _, r := range message {
		if r >= '\u4e00' && r <= '\u9fff' {
			return message
		}
	}
	return "Agent 接入未完成；请查看本机连接日志或重新验证。"
}

func detectObsidian(configured ...string) string {
	if strings.TrimSpace(os.Getenv("MEOW_LAUNCHER_SIMULATE_NO_OBSIDIAN")) == "1" {
		return ""
	}
	candidates := []string{}
	if override := strings.TrimSpace(os.Getenv("MEOW_OBSIDIAN_PATH")); override != "" {
		candidates = append(candidates, override)
	}
	if len(configured) > 0 && strings.TrimSpace(configured[0]) != "" {
		candidates = append(candidates, configured[0])
	}
	if executable, err := exec.LookPath("Obsidian.exe"); err == nil {
		candidates = append(candidates, executable)
	}
	candidates = append(candidates, platformObsidianCandidates()...)
	for _, item := range []struct{ env, rel string }{
		{"LOCALAPPDATA", filepath.Join("Obsidian", "Obsidian.exe")},
		{"LOCALAPPDATA", filepath.Join("Programs", "Obsidian", "Obsidian.exe")},
		{"ProgramFiles", filepath.Join("Obsidian", "Obsidian.exe")},
		{"ProgramFiles(x86)", filepath.Join("Obsidian", "Obsidian.exe")},
	} {
		if base := strings.TrimSpace(os.Getenv(item.env)); base != "" {
			candidates = append(candidates, filepath.Join(base, item.rel))
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

// DetectObsidian performs an explicit local scan and persists a discovered path.
// It does not install software or modify an Obsidian vault.
func (a *App) DetectObsidian() (LauncherState, error) {
	persisted, err := a.loadSettings()
	if err != nil {
		return LauncherState{}, err
	}
	detected := detectObsidian(persisted.ObsidianPath)
	if detected != "" && !strings.EqualFold(detected, persisted.ObsidianPath) {
		persisted.ObsidianPath = detected
		if err := a.saveSettingsState(persisted); err != nil {
			return LauncherState{}, err
		}
	}
	return a.GetState()
}

func (a *App) ChooseObsidianExecutable() (LauncherState, error) {
	if a.ctx == nil {
		return LauncherState{}, errors.New("窗口尚未准备完成")
	}
	selected, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "选择已安装的 Obsidian.exe",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Obsidian", Pattern: "*.exe"}},
	})
	if err != nil {
		return LauncherState{}, err
	}
	if strings.TrimSpace(selected) == "" {
		return a.GetState()
	}
	selected = filepath.Clean(selected)
	info, statErr := os.Stat(selected)
	if statErr != nil || !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(selected), ".exe") {
		return LauncherState{}, errors.New("所选文件不是可用的 Windows 可执行文件")
	}
	state, err := a.loadSettings()
	if err != nil {
		return LauncherState{}, err
	}
	state.ObsidianPath = selected
	if err := a.saveSettingsState(state); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) ChooseDirectory(title string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("窗口尚未准备完成")
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: title})
}

func (a *App) InstallBrain2(root string) (LauncherState, error) {
	if err := brain2.Initialize(root); err != nil {
		return LauncherState{}, err
	}
	if err := a.saveRoot(root); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) AttachBrain2(root string) (LauncherState, error) {
	root = strings.TrimSpace(root)
	if err := brain2.AttachExisting(root); err != nil {
		return LauncherState{}, err
	}
	if err := a.saveRoot(root); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) InitializeLocalDevice(deviceID, root string) (LauncherState, error) {
	deviceID, err := brain2.NormalizeDeviceID(deviceID)
	if err != nil {
		return LauncherState{}, err
	}
	root = strings.TrimSpace(root)
	if err := brain2.AttachExisting(root); err != nil {
		return LauncherState{}, err
	}
	if err := a.saveRoot(root); err != nil {
		return LauncherState{}, err
	}
	state, err := a.loadSettings()
	if err != nil {
		return LauncherState{}, err
	}
	state.LocalDeviceID = deviceID
	state.SelectedDeviceID = deviceID
	if err := a.saveSettingsState(state); err != nil {
		return LauncherState{}, err
	}
	if _, err := brain2.EnsureDeviceRegistry(root, deviceID); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) SetLocalDevice(deviceID string) (LauncherState, error) {
	deviceID, err := brain2.NormalizeDeviceID(deviceID)
	if err != nil {
		return LauncherState{}, err
	}
	state, err := a.loadSettings()
	if err != nil {
		return LauncherState{}, err
	}
	if !brain2.IsWorkspace(state.Brain2Root) {
		return LauncherState{}, errors.New("请先安装或接入 Brain2")
	}
	if _, err := brain2.EnsureDeviceRegistry(state.Brain2Root, deviceID); err != nil {
		return LauncherState{}, err
	}
	state.LocalDeviceID = deviceID
	state.SelectedDeviceID = deviceID
	if err := a.saveSettingsState(state); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) SwitchDevice(deviceID string) (LauncherState, error) {
	deviceID, err := brain2.NormalizeDeviceID(deviceID)
	if err != nil {
		return LauncherState{}, err
	}
	state, err := a.loadSettings()
	if err != nil {
		return LauncherState{}, err
	}
	if !brain2.IsWorkspace(state.Brain2Root) {
		return LauncherState{}, errors.New("请先安装或接入 Brain2")
	}
	if strings.TrimSpace(state.LocalDeviceID) == "" {
		return LauncherState{}, errors.New("请先确认这台机器的设备代码")
	}
	registry, err := brain2.EnsureDeviceRegistry(state.Brain2Root, state.LocalDeviceID)
	if err != nil {
		return LauncherState{}, err
	}
	if _, exists := brain2.DeviceByID(registry, deviceID); !exists {
		return LauncherState{}, errors.New("该设备尚未加入当前 Brain2；请先添加设备")
	}
	state.SelectedDeviceID = deviceID
	if err := a.saveSettingsState(state); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) AddDevice(deviceID string) (LauncherState, error) {
	state, err := a.loadSettings()
	if err != nil {
		return LauncherState{}, err
	}
	if !brain2.IsWorkspace(state.Brain2Root) {
		return LauncherState{}, errors.New("请先安装或接入 Brain2")
	}
	if strings.TrimSpace(state.LocalDeviceID) == "" {
		return LauncherState{}, errors.New("请先确认这台机器的设备代码")
	}
	if _, err := brain2.AddDevice(state.Brain2Root, deviceID, state.LocalDeviceID); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) RemoveDevice(deviceID string) (LauncherState, error) {
	state, err := a.loadSettings()
	if err != nil {
		return LauncherState{}, err
	}
	if !brain2.IsWorkspace(state.Brain2Root) {
		return LauncherState{}, errors.New("请先安装或接入 Brain2")
	}
	if strings.TrimSpace(state.LocalDeviceID) == "" {
		return LauncherState{}, errors.New("请先确认这台机器的设备代码")
	}
	deviceID, err = brain2.NormalizeDeviceID(deviceID)
	if err != nil {
		return LauncherState{}, err
	}
	if _, err := brain2.RemoveDevice(state.Brain2Root, deviceID, state.LocalDeviceID); err != nil {
		return LauncherState{}, err
	}
	if state.SelectedDeviceID == deviceID {
		state.SelectedDeviceID = state.LocalDeviceID
		if err := a.saveSettingsState(state); err != nil {
			return LauncherState{}, err
		}
	}
	return a.GetState()
}

func (a *App) CreateInternalProject(name, goal string) (LauncherState, error) {
	root, err := a.requireRoot()
	if err != nil {
		return LauncherState{}, err
	}
	if err := brain2.CreateInternalProject(root, name, goal); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) AttachExternalProject(name, sourcePath, goal string) (LauncherState, error) {
	root, err := a.requireRoot()
	if err != nil {
		return LauncherState{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(filepath.Clean(sourcePath))
	}
	if err := brain2.RegisterExternalProjectMetadata(root, name, sourcePath, goal); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) ConfirmProjectManifest(projectID, sourcePath string) (LauncherState, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return LauncherState{}, err
	}
	if err := brain2.ConfirmProjectManifestForDevice(root, projectID, sourcePath, deviceID); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) PrepareProjectScan(agentID, scanRoot string) (ProjectScanResult, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return ProjectScanResult{}, err
	}
	task, err := brain2.CreateProjectScanTask(root, scanRoot, agentID)
	if err != nil {
		return ProjectScanResult{}, err
	}
	connection, _, err := brain2.PrepareConnectionWithOptions(root, agentID, "prompt", brain2.ConnectionPromptOptions{DeviceID: deviceID, LocalReady: true, WorkIntent: "brain"})
	if err != nil {
		return ProjectScanResult{}, err
	}
	discoveryScope := "先从 Brain2 的 NAV、项目入口、进度记录、GitHub 交接记录和已有机器登记寻找证据；只沿证据指向的位置核验，不得遍历整块磁盘。"
	if task.DiscoveryMode == "bounded_scan" {
		discoveryScope = "用户授权的兜底扫描范围: " + task.ScanRoot
	}
	prompt := fmt.Sprintf(`这是 Meow Launcher 生成的项目识别任务。先连接指定的协同记忆库，再执行证据驱动的只读项目发现。

记忆库身份：
- brain2_root: %s
- workspace_id: %s
- connection_id: %s
- connection_nonce: %s

扫描任务：
- scan_task_id: %s
- scan_nonce: %s
- agent_id: %s
- 发现模式: %s
- 范围边界: %s

目标：
先读取当前记忆库的项目与治理上下文，避免重复登记；再识别属于代码类、应用类或产品类的项目候选，并把结果回填给 Meow Launcher。普通知识主题、规则集合、纯资料目录和 纯研究主题默认不是本工作流的项目。

执行边界：
1. 调用 brain2_get_bootstrap，传入 connection_id、connection_nonce 作为 nonce、agent_id；确认返回的 workspace_id 与 brain2_root 完全一致。
2. 调用 brain2_health，传入 connection_id；如果身份不一致或健康检查 critical，停止并报告。
3. 只读发现；不得移动、重命名、修改或删除任何文件。Brain2-first 模式不得为了找项目而全盘遍历磁盘，只能沿 Brain2、GitHub 与已知本机路径证据核验。
4. 不要把 node_modules、vendor、dist、build、.git、缓存、临时目录或备份目录识别为项目。
5. Brain2 工作区（brain2_root）是记忆库和证据入口，不等于 Brain2 Manager 产品。只有找到 Manager 的应用入口、独立构建物或产品文档等证据，才能提交 Brain2 Manager 候选；不得把 D:\Brain2 本身命名成 Brain2 Manager。
6. 每个候选必须提供绝对根路径、0-1 confidence、detected_kind、最多 300 字的 suggested_goal 和最多 8 条可验证依据。detected_kind 只能是 code_project、application、product 或 needs_verification；普通研究主题、业务主题、知识分类和内容流程不得提交。
7. 如果两个独立产品共享同一个代码仓库，必须分别提交候选，并在后提交的候选中填写 shared_repository_project_id，指向已登记的仓库项目；这表示平级产品共享代码，不是 parent_project_id 父子关系。例如 Brain2 Manager 可与 Meow Launcher 共享仓库，但二者仍是独立产品。
8. suggested_goal 是 Agent 根据证据提出的用途候选，不是 Launcher 或 Brain2 的判断；用户确认后才会写入项目身份。
9. 不确定时降低 confidence，不要猜测；不要提交文件正文、密钥、隐私内容或长摘要。
10. bounded_scan 模式下候选路径必须位于授权目录内；brain2_first 模式下每个本机路径都必须由 Brain2 或 GitHub 交接证据指向，禁止猜路径。
11. 扫描完成后调用 brain2_submit_project_candidates，传入 scan_task_id、scan_nonce、agent_id 和 candidates。
12. 不得直接修改 .brain2/projects.json、项目资产登记或 Product Registry；用户只会选择是否加入建档，技术字段由后续建档任务调查。
13. 工具提交成功后告诉用户“候选已回填 Launcher”，不要声称项目已建档。Launcher 会自动刷新候选列表。
`, root, task.WorkspaceID, connection.ConnectionID, connection.Nonce, task.TaskID, task.Nonce, task.AgentID, task.DiscoveryMode, discoveryScope)
	return ProjectScanResult{TaskID: task.TaskID, Message: "项目发现 Prompt 已生成；请交给所选 Agent", Prompt: prompt, Brain2Root: root, WorkspaceID: task.WorkspaceID, ScanRoot: task.ScanRoot}, nil
}

func (a *App) CopyText(value string) error {
	if len(value) > 200*1024 {
		return errors.New("复制内容超过 200 KiB 限制")
	}
	if a.ctx == nil {
		return errors.New("窗口尚未准备完成")
	}
	return wailsruntime.ClipboardSetText(a.ctx, value)
}

func (a *App) ConfirmProjectCandidate(candidateID string) (LauncherState, error) {
	root, err := a.requireRoot()
	if err != nil {
		return LauncherState{}, err
	}
	if err := brain2.ConfirmProjectCandidate(root, candidateID); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) IgnoreProjectCandidate(candidateID string) (LauncherState, error) {
	root, err := a.requireRoot()
	if err != nil {
		return LauncherState{}, err
	}
	if err := brain2.IgnoreProjectCandidate(root, candidateID); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) AssignProjectCandidateToParent(candidateID, parentProjectID string) (LauncherState, error) {
	root, err := a.requireRoot()
	if err != nil {
		return LauncherState{}, err
	}
	if err := brain2.AssignProjectCandidateToParent(root, candidateID, parentProjectID); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) PrepareProjectCandidateArchive(agentID, candidateID string) (ProjectArchiveResult, error) {
	result, err := a.PrepareProjectCandidateArchives(agentID, []string{candidateID})
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	if len(result.Items) != 1 {
		return ProjectArchiveResult{}, errors.New("项目建档任务生成结果异常")
	}
	item := result.Items[0]
	return ProjectArchiveResult{TaskID: item.TaskID, Message: "专属建档任务已生成；Agent 完成回执后项目才会进入已登记清单", Prompt: result.Prompt, CandidateID: item.CandidateID, ProjectName: item.ProjectName}, nil
}

func (a *App) PrepareProjectCandidateArchives(agentID string, candidateIDs []string) (ProjectArchiveBatchResult, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return ProjectArchiveBatchResult{}, err
	}
	connection, _, err := brain2.PrepareConnectionWithOptions(root, agentID, "prompt", brain2.ConnectionPromptOptions{DeviceID: deviceID, LocalReady: true, WorkIntent: "brain"})
	if err != nil {
		return ProjectArchiveBatchResult{}, err
	}
	tasks, err := brain2.CreateProjectArchiveTasks(root, candidateIDs, agentID)
	if err != nil {
		return ProjectArchiveBatchResult{}, err
	}
	registry, err := brain2.LoadProjectCandidates(root)
	if err != nil {
		return ProjectArchiveBatchResult{}, err
	}
	byID := make(map[string]brain2.ProjectCandidate, len(registry.Candidates))
	for _, candidate := range registry.Candidates {
		byID[candidate.ID] = candidate
	}
	selected := make([]brain2.ProjectCandidate, 0, len(tasks))
	items := make([]ProjectArchiveBatchItem, 0, len(tasks))
	for _, task := range tasks {
		candidate, ok := byID[task.CandidateID]
		if !ok {
			return ProjectArchiveBatchResult{}, errors.New("项目候选不存在")
		}
		selected = append(selected, candidate)
		items = append(items, ProjectArchiveBatchItem{TaskID: task.TaskID, CandidateID: candidate.ID, ProjectName: candidate.Name})
	}
	prompt := renderProjectArchivePrompt(root, connection, tasks, selected)
	return ProjectArchiveBatchResult{Message: fmt.Sprintf("已生成 %d 个独立建档任务；Agent 必须逐项目提交回执", len(items)), Prompt: prompt, Items: items}, nil
}

func renderProjectArchivePrompt(root string, connection brain2.ConnectionTask, tasks []brain2.ProjectArchiveTask, candidates []brain2.ProjectCandidate) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, `这是 Meow Launcher 根据用户选择生成的一次性项目建档任务，本次包含 %d 个候选。用户已经决定为下面列出的每个候选建立档案，无需让用户确认技术字段。

连接身份：
- brain2_root: %s
- workspace_id: %s
- connection_id: %s
- connection_nonce: %s
- agent_id: %s

建档授权（每一项都是独立任务，必须分别处理）：
`, len(tasks), root, connection.WorkspaceID, connection.ConnectionID, connection.Nonce, connection.AgentID)
	for index, task := range tasks {
		candidate := candidates[index]
		fmt.Fprintf(&builder, `
[%d]
- archive_task_id: %s
- archive_nonce: %s
- candidate_id: %s
- candidate_name: %s
- 唯一授权本机源路径: %s
- Agent 提出的用途候选: %s
`, index+1, task.TaskID, task.Nonce, candidate.ID, candidate.Name, task.AllowedSourcePath, candidate.SuggestedGoal)
		if candidate.SharedRepositoryProjectID != "" {
			fmt.Fprintf(&builder, "- 共享代码仓库项目: %s（独立产品，不是父子项目）\n", candidate.SharedRepositoryProjectID)
		}
	}
	builder.WriteString(`
你的职责：
1. 先对本批次所有任务调用 brain2_get_bootstrap 和 brain2_health，核验返回的 Brain2 身份与连接参数一致；读取 Brain2 的 NAV、项目入口和相关进度，优先复用已有项目 README。
2. 对每个候选分别只读核验根目录的代码/应用证据；如有 Git，核验 remote、当前或默认分支和仓库身份。不得提交密钥、文件正文或隐私数据。
3. 对每个候选分别判断 entity_kind：code_project、application、product；证据不足填 needs_verification。分别判断 lifecycle_stage：discovery、active、paused、completed、archived；不确定填 needs_verification。
4. 对每个候选分别确定 brain2_entry：优先指向 Brain2 中已有的项目 README。若确实没有入口，可留空，由协议创建 02 项目/<项目名>/README.md；不得指定一个尚不存在的其他位置。
5. 候选带 shared_repository_project_id 时，必须把相同值原样写入 archive；它表示独立产品共享仓库，不得改写为 parent_project_id，也不得因此合并产品档案。
6. A/B 机等额外本地路径可写入各自 additional_paths；当前设备无法核验的路径必须标 status=unknown，并写入 needs_verification。
7. GitHub 仓库只接受无凭据的 https://github.com/<owner>/<repo>；无法核验时留空并写 needs_verification，不得猜测。
8. 必须对每个候选逐项目分别调用 brain2_submit_project_archive，分别传入对应的 archive_task_id、archive_nonce、agent_id 和 archive 对象；不得把多个候选合并成一个 archive，也不得只提交一次代表整批。
9. 每个 MCP 返回 status=verified 后，才可报告对应项目“项目档案已建立”，并分别报告 project_id、changed_paths 和 needs_verification。某个项目失败时必须如实报告该项目失败原因；可以继续处理其他独立任务，但不得把部分成功说成整批成功。
10. 用户授权的是这些候选的非破坏性建档，不包括移动目录、合并项目、创建/公开 GitHub 仓库、扩大扫描范围或修改源码；这些动作仍需另行询问。
`)
	return builder.String()
}

func (a *App) OpenProtectorProjectPicker() (ProtectorCatalogLaunchResult, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return ProtectorCatalogLaunchResult{}, err
	}
	config, err := brain2.LoadConfig(root)
	if err != nil {
		return ProtectorCatalogLaunchResult{}, err
	}
	projects, err := brain2.LoadProjects(root)
	if err != nil {
		return ProtectorCatalogLaunchResult{}, err
	}
	machineName, err := localMachineName(root, deviceID)
	if err != nil {
		return ProtectorCatalogLaunchResult{}, err
	}
	confirmed := make([]brain2.Project, 0, len(projects.Projects))
	for _, project := range projects.Projects {
		if productlaunch.ProjectAvailableOnDevice(project, deviceID, machineName) {
			confirmed = append(confirmed, project)
		}
	}
	dataRoot, err := settings.DataRoot()
	if err != nil {
		return ProtectorCatalogLaunchResult{}, err
	}
	productRoot := strings.TrimSpace(os.Getenv("MEOW_PROTECTOR_DEV_ROOT"))
	prepared, err := productlaunch.PrepareProtectorCatalogLaunch(dataRoot, config.InstanceID, productRoot, deviceID, machineName, confirmed, time.Now())
	if err != nil {
		return ProtectorCatalogLaunchResult{}, err
	}
	command := exec.Command(prepared.Command, prepared.Args...)
	command.Dir = filepath.Clean(productRoot)
	command.Env = os.Environ()
	if err := command.Start(); err != nil {
		return ProtectorCatalogLaunchResult{}, fmt.Errorf("启动 Protector 项目选择器失败: %w", err)
	}
	_ = command.Process.Release()
	handshake, err := productlaunch.AwaitCatalogHandshake(prepared, 5*time.Second)
	if err != nil {
		return ProtectorCatalogLaunchResult{}, err
	}
	return ProtectorCatalogLaunchResult{
		Status: "accepted", Message: fmt.Sprintf("Protector 已打开；可选择 %d 个 Brain2 已确认项目", handshake.ProjectCount),
		CatalogID: prepared.CatalogID, ProjectCount: handshake.ProjectCount,
		ProductVersion: handshake.ProductVersion, ExpiresAt: prepared.ExpiresAt,
	}, nil
}

func (a *App) OpenDevHelper(projectID string) (HelperLaunchResult, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return HelperLaunchResult{}, err
	}
	config, err := brain2.LoadConfig(root)
	if err != nil {
		return HelperLaunchResult{}, err
	}
	projects, err := brain2.LoadProjects(root)
	if err != nil {
		return HelperLaunchResult{}, err
	}
	machineName, err := localMachineName(root, deviceID)
	if err != nil {
		return HelperLaunchResult{}, err
	}
	projectID = strings.TrimSpace(projectID)
	var selected *brain2.Project
	for index := range projects.Projects {
		if projects.Projects[index].ID == projectID {
			selected = &projects.Projects[index]
			break
		}
	}
	if selected == nil {
		return HelperLaunchResult{}, errors.New("未找到要授权给 Dev Helper 的已登记项目")
	}
	dataRoot, err := settings.DataRoot()
	if err != nil {
		return HelperLaunchResult{}, err
	}
	productRoot := strings.TrimSpace(os.Getenv("MEOW_DEV_HELPER_ROOT"))
	prepared, err := productlaunch.PrepareHelperLaunch(dataRoot, config.InstanceID, productRoot, deviceID, machineName, *selected, time.Now())
	if err != nil {
		return HelperLaunchResult{}, err
	}
	command := exec.Command(prepared.Command, prepared.Args...)
	command.Dir = filepath.Clean(productRoot)
	command.Env = os.Environ()
	if err := command.Start(); err != nil {
		return HelperLaunchResult{}, fmt.Errorf("启动 Dev Helper 失败: %w", err)
	}
	_ = command.Process.Release()
	handshake, err := productlaunch.AwaitHelperHandshake(prepared, 5*time.Second)
	if err != nil {
		return HelperLaunchResult{}, err
	}
	return HelperLaunchResult{
		Status: "accepted", Message: fmt.Sprintf("Dev Helper 已打开并获得项目“%s”的本次授权", selected.Name),
		TaskID: prepared.TaskID, ProjectID: selected.ID, ProductVersion: handshake.ProductVersion,
		AcceptedScopes: append([]string(nil), handshake.AcceptedScopes...), ExpiresAt: prepared.ExpiresAt,
	}, nil
}

func (a *App) PrepareCodexRuntime() (LauncherState, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return LauncherState{}, err
	}
	if err := brain2.PrepareRuntimeForDevice(root, deviceID, "codex"); err != nil {
		return LauncherState{}, err
	}
	return a.GetState()
}

func (a *App) DetectAgents() ([]agentconnect.Detection, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	_, rootErr := a.requireRoot()
	executable, exeErr := os.Executable()
	if rootErr != nil || exeErr != nil {
		return agentconnect.DetectAll(ctx), nil
	}
	registryPath, registryErr := a.productRegistryPath()
	if registryErr != nil {
		return nil, registryErr
	}
	detections := agentconnect.DetectAllForWorkspace(ctx, registryPath, executable)
	persisted, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	for index := range detections {
		ignoredVersion := persisted.IgnoredConnectorVersions[detections[index].ID]
		if detections[index].MCPStatus == "update_available" && ignoredVersion == detections[index].MCPAvailable {
			detections[index].MCPStatus = "update_ignored"
			detections[index].MCPMessage = "已忽略此版本；旧连接器仍可继续使用"
		}
	}
	return detections, nil
}

func (a *App) IgnoreConnectorUpdate(agentID string) error {
	if _, ok := brain2.AgentDefinitionByID(agentID); !ok {
		return fmt.Errorf("不支持的 Agent %q", agentID)
	}
	state, err := a.loadSettings()
	if err != nil {
		return err
	}
	if state.IgnoredConnectorVersions == nil {
		state.IgnoredConnectorVersions = map[string]string{}
	}
	state.IgnoredConnectorVersions[agentID] = agentconnect.MCPHostVersion
	return a.saveSettingsState(state)
}

func (a *App) RestoreConnectorUpdate(agentID string) error {
	state, err := a.loadSettings()
	if err != nil {
		return err
	}
	delete(state.IgnoredConnectorVersions, agentID)
	return a.saveSettingsState(state)
}

func (a *App) PrepareAgentPrompt(agentID, workIntent, projectID, projectName, projectGoal string) (agentconnect.Result, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return agentconnect.Result{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return agentconnect.Result{}, err
	}
	registryPath, err := a.productRegistryPath()
	if err != nil {
		return agentconnect.Result{}, err
	}
	result, err := agentconnect.PreparePrompt(root, registryPath, agentID, executable, brain2ConnectSkill, agentconnect.PromptOptions{
		DeviceID: deviceID, WorkIntent: workIntent, ProjectID: projectID, ProjectName: projectName, ProjectGoal: projectGoal,
	})
	if err != nil {
		return agentconnect.Result{}, err
	}
	return result, nil
}

func (a *App) PrepareProjectArchivePrompt(agentID, projectID string) (agentconnect.Result, error) {
	root, err := a.requireRoot()
	if err != nil {
		return agentconnect.Result{}, err
	}
	projects, err := brain2.LoadProjects(root)
	if err != nil {
		return agentconnect.Result{}, err
	}
	var selected *brain2.Project
	for index := range projects.Projects {
		if projects.Projects[index].ID == strings.TrimSpace(projectID) {
			selected = &projects.Projects[index]
			break
		}
	}
	if selected == nil {
		return agentconnect.Result{}, errors.New("项目不存在")
	}
	result, err := a.PrepareAgentPrompt(agentID, "existing_project", selected.ID, selected.Name, selected.Goal)
	if err != nil {
		return agentconnect.Result{}, err
	}
	result.Prompt += fmt.Sprintf(`

本次任务仅制作“项目档案草稿”，不要直接创建或修改文件。
项目：%s
当前登记路径：%s

请只读核查并在对话中提交草稿，至少说明：
1. 项目用途与当前状态；
2. 代码、工程、数据和主要成果分别位于哪里；
3. 哪些路径是事实，哪些只是推测或本机缺失；
4. 建议新增或更新的项目说明文件位置；
5. 拟写入的 Markdown 全文与证据来源。

边界：Launcher 只提供机器登记；你负责收集证据和起草。先把草稿与拟修改文件清单交给用户确认，未获明确授权不得写入、移动、重命名或整理任何内容。
`, selected.Name, selected.SourcePathValue())
	result.Message = "项目补档草稿任务已生成；Agent 只会提案，是否写入由你确认"
	return result, nil
}

func (a *App) QuickConnectAgent(agentID string) (agentconnect.Result, error) {
	return a.connectAgent(agentID, false)
}

func (a *App) RebindAgent(agentID string) (agentconnect.Result, error) {
	return a.connectAgent(agentID, true)
}

func (a *App) RollbackAgentConnector(agentID string) (agentconnect.Result, error) {
	root, _, err := a.requireLocalRoot()
	if err != nil {
		return agentconnect.Result{}, err
	}
	registryPath, err := a.productRegistryPath()
	if err != nil {
		return agentconnect.Result{}, err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	report := func(progress agentconnect.Progress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "agent-connect-progress", progress)
		}
	}
	return agentconnect.Rollback(ctx, root, registryPath, agentID, report)
}

func (a *App) connectAgent(agentID string, replaceExistingRoot bool) (agentconnect.Result, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return agentconnect.Result{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return agentconnect.Result{}, err
	}
	registryPath, err := a.productRegistryPath()
	if err != nil {
		return agentconnect.Result{}, err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	report := func(progress agentconnect.Progress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "agent-connect-progress", progress)
		}
	}
	var result agentconnect.Result
	if replaceExistingRoot {
		result, err = agentconnect.RebindForDevice(ctx, root, registryPath, deviceID, agentID, executable, brain2ConnectSkill, report)
	} else {
		result, err = agentconnect.QuickConnectForDevice(ctx, root, registryPath, deviceID, agentID, executable, brain2ConnectSkill, report)
	}
	if err != nil {
		return agentconnect.Result{}, err
	}
	return result, nil
}

func (a *App) VerifyAgent(agentID string) (agentconnect.Result, error) {
	root, deviceID, err := a.requireLocalRoot()
	if err != nil {
		return agentconnect.Result{}, err
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	report := func(progress agentconnect.Progress) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "agent-connect-progress", progress)
		}
	}
	return agentconnect.VerifyAgentForDevice(ctx, root, deviceID, agentID, report)
}

func (a *App) RunHealthCheck() (brain2.HealthReport, error) {
	root, err := a.requireRoot()
	if err != nil {
		return brain2.HealthReport{}, err
	}
	return brain2.Check(root)
}

func (a *App) PrepareHealthAgentPrompt() (string, error) {
	root, err := a.requireRoot()
	if err != nil {
		return "", err
	}
	report, err := brain2.Check(root)
	if err != nil {
		return "", err
	}
	var findings strings.Builder
	var categories strings.Builder
	for _, category := range report.Categories {
		fmt.Fprintf(&categories, "- %s：%d 通过，%d 候选，%d 警告，%d 错误\n", category.Category, category.OK, category.Info, category.Warning, category.Error)
	}
	findingCount := 0
	for _, check := range report.Checks {
		if check.Status == "ok" {
			continue
		}
		if findingCount == 40 {
			fmt.Fprintf(&findings, "- 其余结果请按需读取检查报告；Prompt 不重复装入完整报告。\n")
			break
		}
		fmt.Fprintf(&findings, "- [%s] %s：%s（%s）\n", check.Status, check.Name, check.Message, check.Path)
		findingCount++
	}
	if findings.Len() == 0 {
		findings.WriteString("- 当前确定性检查没有发现需要关注的项目。\n")
	}
	prompt := fmt.Sprintf(`读取 Brain2，并分析下面这份由 Meow Launcher 生成的确定性检查结果。

Brain2 根目录：%s
检查报告：%s

分层摘要：
%s
检测事实：
%s
边界：
1. 先读取 Brain2 治理规则和检查报告原文，再判断哪些项目真的需要处理。
2. 断链、路径失效、身份不一致属于代码检测事实；“孤立候选”不等于错误。
3. 不要自动移动、重命名、删除或改写用户内容。
4. 先向用户给出处理建议和拟修改文件清单；获得明确授权后才能执行。
5. 完成后重新运行确定性检查，并报告仍未解决的项目。
`, root, filepath.Join(root, ".brain2", "health-report.json"), categories.String(), findings.String())
	return prompt, nil
}

func (a *App) PrepareObsidianInstallPrompt(installPath string) (string, error) {
	installPath = strings.TrimSpace(installPath)
	if installPath == "" {
		return "", errors.New("请先选择希望安装 Obsidian 的位置")
	}
	prompt := fmt.Sprintf(`请帮我在这台 Windows 电脑上安装 Obsidian。

用户期望位置：%s
官方下载页：https://obsidian.md/download
官方 GitHub 发布页：https://github.com/obsidianmd/obsidian-releases/releases/latest

执行边界：
1. 只从上述 Obsidian 官方入口获取安装程序，不使用第三方下载站。
2. 先检查系统是否已经安装 Obsidian；已安装时报告实际路径，不重复安装。
3. 先确认当前官方安装器是否支持自定义安装位置。若不支持，不要静默安装到其他目录，先向我说明默认位置并询问是否继续。
4. 不修改我的 Obsidian Vault、笔记、插件、主题和现有配置。
5. 需要管理员权限、改变系统级设置或安装到与上述路径不同的位置时，必须先询问。
6. 安装后启动一次 Obsidian，确认可以正常打开，并报告实际 Obsidian.exe 路径；不要替我创建或整理内容仓库。
`, filepath.Clean(installPath))
	return prompt, nil
}

func (a *App) OpenBrain2Folder() error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	return exec.Command("explorer.exe", root).Start()
}

func (a *App) OpenBrain2InObsidian() error {
	root, err := a.requireRoot()
	if err != nil {
		return err
	}
	state, stateErr := a.loadSettings()
	if stateErr != nil {
		return stateErr
	}
	obsidianPath := detectObsidian(state.ObsidianPath)
	if obsidianPath == "" {
		return errors.New("尚未检测到 Obsidian；请先在“开始使用”中生成 Agent 安装任务")
	}
	return exec.Command(obsidianPath, "obsidian://open?path="+url.QueryEscape(root)).Start()
}

func (a *App) WindowMinimise() {
	if a.ctx != nil {
		wailsruntime.WindowMinimise(a.ctx)
	}
}

func (a *App) WindowToggleMaximise() {
	if a.ctx != nil {
		wailsruntime.WindowToggleMaximise(a.ctx)
	}
}

func (a *App) WindowClose() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

func (a *App) requireRoot() (string, error) {
	state, err := a.loadSettings()
	if err != nil {
		return "", err
	}
	if !brain2.IsWorkspace(state.Brain2Root) {
		return "", errors.New("请先安装或接入 Brain2")
	}
	return state.Brain2Root, nil
}

func (a *App) requireLocalRoot() (string, string, error) {
	state, err := a.loadSettings()
	if err != nil {
		return "", "", err
	}
	if !brain2.IsWorkspace(state.Brain2Root) {
		return "", "", errors.New("请先安装或接入 Brain2")
	}
	localDeviceID := strings.ToUpper(strings.TrimSpace(state.LocalDeviceID))
	if localDeviceID == "" {
		return "", "", errors.New("请先确认这台机器的设备代码")
	}
	selectedDeviceID := strings.ToUpper(strings.TrimSpace(state.SelectedDeviceID))
	if selectedDeviceID == "" {
		return "", "", errors.New("请先选择当前查看设备")
	}
	if selectedDeviceID != localDeviceID {
		return "", "", fmt.Errorf("当前正在查看设备 %s 的远程记录；请切回设备 %s（本机）后执行此操作", selectedDeviceID, localDeviceID)
	}
	return state.Brain2Root, localDeviceID, nil
}

func localMachineName(root, deviceID string) (string, error) {
	registry, err := brain2.LoadDevicesOrEmpty(root)
	if err != nil {
		return "", err
	}
	device, exists := brain2.DeviceByID(registry, deviceID)
	if !exists {
		return "", fmt.Errorf("Brain2 设备 %s 尚未登记", deviceID)
	}
	return strings.TrimSpace(device.MachineName), nil
}

func (a *App) loadSettings() (settings.State, error) {
	if a.statePath != "" {
		return settings.LoadFrom(a.statePath)
	}
	return settings.Load()
}

func (a *App) saveRoot(root string) error {
	if err := a.syncProductRegistry(root); err != nil {
		return err
	}
	if a.statePath != "" {
		return settings.SaveTo(a.statePath, root)
	}
	return settings.Save(root)
}

func (a *App) saveSettingsState(state settings.State) error {
	if a.statePath != "" {
		return settings.SaveStateTo(a.statePath, state)
	}
	return settings.SaveState(state)
}

func (a *App) syncProductRegistry(root string) error {
	cfg, err := brain2.EnsureInstanceID(root)
	if err != nil {
		return err
	}
	registryPath, err := a.productRegistryPath()
	if err != nil {
		return err
	}
	if err := productregistry.UpsertBrain2(registryPath, root, cfg.InstanceID); err != nil {
		return err
	}
	return nil
}

func (a *App) productRegistryPath() (string, error) {
	if a.registryPath != "" {
		if !filepath.IsAbs(a.registryPath) {
			return "", errors.New("Launcher 产品注册表路径必须是绝对路径")
		}
		return filepath.Clean(a.registryPath), nil
	}
	return settings.ProductRegistryPath()
}
