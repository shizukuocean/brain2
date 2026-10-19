export interface ProjectSummary {
  id: string
  name: string
  mode: 'internal' | 'external' | string
  status: string
  source_path: string
  manifest_path: string
  asset_status: string
  path_status: string
  truth_status: string
  shared_repository_project_id?: string
  machine_record: boolean
  protector_eligible: boolean
  helper_eligible: boolean
}

export interface LauncherState {
  brain2_root: string
  workspace: boolean
  project_count: number
  agent_count: number
  runtime_ready: boolean
  health_status: string
  health_score: number
  projects: ProjectSummary[]
  agents: AgentSummary[]
  launcher_build: string
  launcher_profile: string
  obsidian_ready: boolean
  obsidian_path: string
  acceptance_mode: string
  project_candidates: ProjectCandidate[]
  protector_available: boolean
  helper_available: boolean
	local_device_id: string
	selected_device_id: string
	selected_device_local: boolean
	device_setup_required: boolean
	local_device_source: string
	devices: DeviceSummary[]
}

export interface DeviceSummary {
  id: 'A' | 'B' | string
  name: string
  brain2_path: string
  machine_name: string
  local: boolean
  status: string
}

export interface ProtectorCatalogLaunchResult {
  status: string
  message: string
  catalog_id: string
  project_count: number
  product_version: string
  expires_at: string
}

export interface HelperLaunchResult {
  status: string
  message: string
  task_id: string
  project_id: string
  product_version: string
  accepted_scopes: string[]
  expires_at: string
}

export interface ProjectCandidate {
  id: string
  scan_task_id: string
  agent_id: string
  name: string
  root_path: string
  suggested_goal: string
  confidence: number
  evidence: string[]
  detected_kind: string
  suggested_consumers: string[]
  parent_project_id?: string
  shared_repository_project_id?: string
  status: 'pending' | 'selected_for_archiving' | 'assigned_to_parent' | 'archived' | 'ignored' | string
  submitted_at: string
}

export interface ProjectScanResult {
  task_id: string
  message: string
  prompt: string
  brain2_root: string
  workspace_id: string
  scan_root: string
}

export interface ProjectArchiveResult {
  task_id: string
  message: string
  prompt: string
  candidate_id: string
  project_name: string
}

export interface ProjectArchiveBatchItem {
  task_id: string
  candidate_id: string
  project_name: string
}

export interface ProjectArchiveBatchResult {
  message: string
  prompt: string
  items: ProjectArchiveBatchItem[]
}

export interface AgentSummary {
  id: string
  name: string
  surface: string
  status: string
  mode: string
  connected_at: string
  last_error: string
}

export interface AgentDetection {
  id: string
  name: string
  surface: 'cli' | 'desktop' | string
  detected: boolean
  quick_connect: boolean
  prompt_connect: boolean
  executable: string
  version: string
  status: string
  message: string
  mcp_status: string
  mcp_message: string
  mcp_version: string
  mcp_available_version: string
  rollback_available: boolean
}

export interface AgentConnectResult {
  agent_id: string
  status: string
  connection_id: string
  message: string
  prompt: string
  receipt_path: string
  changed_paths: string[]
  restart_required: boolean
  skill_updated: boolean
  host_updated: boolean
  config_updated: boolean
  skill_conflict: boolean
  verification_prompt: string
}

export interface HealthCheck {
  name: string
  status: string
  message: string
  path: string
  category?: string
}

export interface HealthCategorySummary {
  category: string
  status: string
  ok: number
  warning: number
  error: number
  info: number
}

export interface HealthReport {
  schema_version: string
  generated_at: string
  overall_status: string
  score: number
  brain2_root: string
  workspace_id: string
  mcp_host_version?: string
  mcp_instance_id?: string
  mcp_started_at?: string
  findings_total: number
  returned_checks: number
  checks_truncated: boolean
  categories: HealthCategorySummary[]
  checks: HealthCheck[]
}

export interface ApplicationRelease {
  product_id: string
  channel: string
  version: string
  published_at: string
  release_notes: string
  minimum_launcher_version: string
  mandatory: boolean
}

export interface ApplicationUpdateResult {
  status: 'not_checked' | 'source_not_configured' | 'check_failed' | 'manifest_invalid' | 'product_not_found' | 'current' | 'update_available'
  error_code?: string
  message: string
  current_version: string
  checked_at: string
  release?: ApplicationRelease
}

interface AppBridge {
  GetState(): Promise<LauncherState>
  CheckApplicationUpdate(): Promise<ApplicationUpdateResult>
  OpenApplicationUpdate(): Promise<void>
  ChooseDirectory(title: string): Promise<string>
  InstallBrain2(root: string): Promise<LauncherState>
  AttachBrain2(root: string): Promise<LauncherState>
	InitializeLocalDevice(deviceID: string, root: string): Promise<LauncherState>
	SwitchDevice(deviceID: string): Promise<LauncherState>
	AddDevice(deviceID: string): Promise<LauncherState>
	RemoveDevice(deviceID: string): Promise<LauncherState>
	SetLocalDevice(deviceID: string): Promise<LauncherState>
  CreateInternalProject(name: string, goal: string): Promise<LauncherState>
  AttachExternalProject(name: string, sourcePath: string, goal: string): Promise<LauncherState>
  ConfirmProjectManifest(projectID: string, sourcePath: string): Promise<LauncherState>
  PrepareProjectScan(agentID: string, scanRoot: string): Promise<ProjectScanResult>
  CopyText(value: string): Promise<void>
  ConfirmProjectCandidate(candidateID: string): Promise<LauncherState>
  IgnoreProjectCandidate(candidateID: string): Promise<LauncherState>
  AssignProjectCandidateToParent(candidateID: string, parentProjectID: string): Promise<LauncherState>
  PrepareProjectCandidateArchive(agentID: string, candidateID: string): Promise<ProjectArchiveResult>
  PrepareProjectCandidateArchives(agentID: string, candidateIDs: string[]): Promise<ProjectArchiveBatchResult>
  OpenProtectorProjectPicker(): Promise<ProtectorCatalogLaunchResult>
  OpenDevHelper(projectID: string): Promise<HelperLaunchResult>
  PrepareCodexRuntime(): Promise<LauncherState>
  DetectAgents(): Promise<AgentDetection[]>
  IgnoreConnectorUpdate(agentID: string): Promise<void>
  RestoreConnectorUpdate(agentID: string): Promise<void>
  PrepareAgentPrompt(agentID: string, workIntent: string, projectID: string, projectName: string, projectGoal: string): Promise<AgentConnectResult>
  PrepareProjectArchivePrompt(agentID: string, projectID: string): Promise<AgentConnectResult>
  QuickConnectAgent(agentID: string): Promise<AgentConnectResult>
  RebindAgent(agentID: string): Promise<AgentConnectResult>
  RollbackAgentConnector(agentID: string): Promise<AgentConnectResult>
  VerifyAgent(agentID: string): Promise<AgentConnectResult>
  RunHealthCheck(): Promise<HealthReport>
  PrepareHealthAgentPrompt(): Promise<string>
  PrepareObsidianInstallPrompt(installPath: string): Promise<string>
  DetectObsidian(): Promise<LauncherState>
  ChooseObsidianExecutable(): Promise<LauncherState>
  OpenBrain2Folder(): Promise<void>
  OpenBrain2InObsidian(): Promise<void>
  WindowMinimise(): Promise<void>
  WindowToggleMaximise(): Promise<void>
  WindowClose(): Promise<void>
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: AppBridge
      }
    }
  }
}

function bridge(): AppBridge {
  const app = window.go?.main?.App
  if (!app) {
    throw new Error('当前页面不在 Meow Launcher 桌面运行环境中')
  }
  return app
}

export const backend = {
  state: () => bridge().GetState(),
  checkApplicationUpdate: () => bridge().CheckApplicationUpdate(),
  openApplicationUpdate: () => bridge().OpenApplicationUpdate(),
  chooseDirectory: (title: string) => bridge().ChooseDirectory(title),
  installBrain2: (root: string) => bridge().InstallBrain2(root),
  attachBrain2: (root: string) => bridge().AttachBrain2(root),
	initializeLocalDevice: (deviceID: string, root: string) => bridge().InitializeLocalDevice(deviceID, root),
	switchDevice: (deviceID: string) => bridge().SwitchDevice(deviceID),
	addDevice: (deviceID: string) => bridge().AddDevice(deviceID),
	removeDevice: (deviceID: string) => bridge().RemoveDevice(deviceID),
	setLocalDevice: (deviceID: string) => bridge().SetLocalDevice(deviceID),
  createInternalProject: (name: string, goal: string) => bridge().CreateInternalProject(name, goal),
  attachExternalProject: (name: string, sourcePath: string, goal: string) =>
    bridge().AttachExternalProject(name, sourcePath, goal),
  confirmProjectManifest: (projectID: string, sourcePath: string) => bridge().ConfirmProjectManifest(projectID, sourcePath),
  prepareProjectScan: (agentID: string, scanRoot: string) => bridge().PrepareProjectScan(agentID, scanRoot),
  copyText: (value: string) => bridge().CopyText(value),
  confirmProjectCandidate: (candidateID: string) => bridge().ConfirmProjectCandidate(candidateID),
  ignoreProjectCandidate: (candidateID: string) => bridge().IgnoreProjectCandidate(candidateID),
  assignProjectCandidateToParent: (candidateID: string, parentProjectID: string) => bridge().AssignProjectCandidateToParent(candidateID, parentProjectID),
  prepareProjectCandidateArchive: (agentID: string, candidateID: string) => bridge().PrepareProjectCandidateArchive(agentID, candidateID),
  prepareProjectCandidateArchives: (agentID: string, candidateIDs: string[]) => bridge().PrepareProjectCandidateArchives(agentID, candidateIDs),
  openProtectorProjectPicker: () => bridge().OpenProtectorProjectPicker(),
  openDevHelper: (projectID: string) => bridge().OpenDevHelper(projectID),
  prepareCodexRuntime: () => bridge().PrepareCodexRuntime(),
  detectAgents: () => bridge().DetectAgents(),
  ignoreConnectorUpdate: (agentID: string) => bridge().IgnoreConnectorUpdate(agentID),
  restoreConnectorUpdate: (agentID: string) => bridge().RestoreConnectorUpdate(agentID),
  prepareAgentPrompt: (agentID: string, workIntent: string, projectID: string, projectName: string, projectGoal: string) =>
    bridge().PrepareAgentPrompt(agentID, workIntent, projectID, projectName, projectGoal),
  prepareProjectArchivePrompt: (agentID: string, projectID: string) => bridge().PrepareProjectArchivePrompt(agentID, projectID),
  quickConnectAgent: (agentID: string) => bridge().QuickConnectAgent(agentID),
  rebindAgent: (agentID: string) => bridge().RebindAgent(agentID),
  rollbackAgentConnector: (agentID: string) => bridge().RollbackAgentConnector(agentID),
  verifyAgent: (agentID: string) => bridge().VerifyAgent(agentID),
  runHealthCheck: () => bridge().RunHealthCheck(),
  prepareHealthAgentPrompt: () => bridge().PrepareHealthAgentPrompt(),
  prepareObsidianInstallPrompt: (installPath: string) => bridge().PrepareObsidianInstallPrompt(installPath),
  detectObsidian: () => bridge().DetectObsidian(),
  chooseObsidianExecutable: () => bridge().ChooseObsidianExecutable(),
  openFolder: () => bridge().OpenBrain2Folder(),
  openObsidian: () => bridge().OpenBrain2InObsidian(),
  minimise: () => bridge().WindowMinimise(),
  maximise: () => bridge().WindowToggleMaximise(),
  close: () => bridge().WindowClose(),
}

export function errorMessage(error: unknown): string {
	const raw = error instanceof Error ? error.message : typeof error === 'string' ? error : ''
	const lower = raw.toLowerCase()
	if (lower.includes('health needs_attention') && lower.includes('without claiming a successful handshake')) {
		return '健康检查有需要处理的项目；受控归档已完成，但本次 Agent 握手未成功。'
	}
	if (lower.includes('network timeout') || lower.includes('timed out') || lower.includes('timeout')) {
		return '网络连接超时，请稍后重试。'
	}
	if (raw) {
		if ([...raw].some((character) => character >= '\u4e00' && character <= '\u9fff')) return raw
		return '操作没有完成，请检查本机连接状态后重试。'
	}
  return '操作失败，请检查 Brain2 状态后重试'
}
