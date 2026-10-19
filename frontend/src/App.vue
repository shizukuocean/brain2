<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { backend, errorMessage, type AgentDetection, type ApplicationUpdateResult, type HealthReport, type LauncherState, type ProjectScanResult } from './backend'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { hermesIcon } from './hermes-icon'

type ViewName = 'product' | 'projects' | 'runtime' | 'health' | 'settings'
type Locale = 'zh' | 'en'
type AgentProgress = { agent_id: string; phase: string; title: string; detail: string }
type UpdateSummary = { skill: string; host: string; config: string; verification: string; conflict?: boolean }
type PromptModalState = { eyebrow: string; title: string; subtitle: string; prompt: string; copied: boolean; updateSummary?: UpdateSummary }
type BrainAttachConfirmationState = { root: string }
type HomeStageID = 'memory' | 'obsidian' | 'agent' | 'project' | 'ready'
type LinkedProductID = 'helper' | 'protector' | 'blueprint'

const savedLocale = window.localStorage.getItem('meow-launcher-locale')
const locale = ref<Locale>(savedLocale === 'en' || savedLocale === 'zh'
	? savedLocale
	: window.navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en')

const copy = {
	zh: {
		currentDevice: '当前设备',
		addDevice: '添加设备',
		manage: '管理',
		localIdentity: '本机身份',
		waitingIdentity: '等待 Launcher 同步身份',
		connected: '已连接',
		waitingConnection: '待连接',
		refresh: '刷新状态',
		homeEyebrow: 'BRAIN2 / 开始使用',
		homeTitle: '开始使用 Brain2',
		homeIntro: '完成当前一步，即可继续。',
		requiredApp: '前置应用',
		localMemory: '协同记忆库',
		agent: 'Agent',
		dailyUse: '日常使用',
		projectDiscovery: '项目识别',
		workspace: '工作区',
		projectLinks: '已连项目',
		agents: 'Agent',
		checks: '检查',
		projectEyebrow: 'BRAIN2 / 项目连接',
		projectTitle: '项目连接',
		projectIntro: '由 Agent 识别项目；你只确认归属与授权。',
		howItWorks: '工作流程',
		singleStep: '仅需一步',
		promptReady: '任务已生成',
		waitingForYou: '等待你确认',
		archiveTasks: '建档任务',
		registeredAssets: '已登记项目',
		runtimeEyebrow: 'BRAIN2 / AGENT 连接',
		runtimeTitle: 'Agent 连接',
		runtimeIntro: '检测并连接本机 Agent，选择工作入口后生成受控 Prompt。',
		workEntry: '工作入口',
		detected: '已检测',
		connectorInstalled: '连接器已安装',
		memoryVerified: '当前记忆库已验证',
		promptAvailable: 'Prompt 可随时使用',
		healthEyebrow: 'BRAIN2 / 检查与维护',
		healthTitle: '检查与维护',
		healthIntro: '查看本机连接、项目登记和内容状态。',
		settingsEyebrow: 'BRAIN2 / 设置',
		settingsTitle: '设置',
		settingsIntro: '管理设备、记忆库、Agent 权限、诊断与更新。',
		deviceManagement: '设备管理',
		currentMemory: '当前记忆库',
		multiMemory: '更多记忆库 · 高级',
		obsidian: 'Obsidian',
		agentAccess: 'Agent 权限',
		diagnostics: '诊断',
	},
	en: {
		currentDevice: 'Current device',
		addDevice: 'Add device',
		manage: 'Manage',
		localIdentity: 'Local identity',
		waitingIdentity: 'Waiting for Launcher identity',
		connected: 'Connected',
		waitingConnection: 'Not connected',
		refresh: 'Refresh',
		homeEyebrow: 'BRAIN2 / GETTING STARTED',
		homeTitle: 'Get started with Brain2',
		homeIntro: 'Complete the current step to continue.',
		requiredApp: 'Required app',
		localMemory: 'Local memory',
		agent: 'Agent',
		dailyUse: 'Daily use',
		projectDiscovery: 'Project discovery',
		workspace: 'Workspace',
		projectLinks: 'Project links',
		agents: 'Agents',
		checks: 'Checks',
		projectEyebrow: 'BRAIN2 / PROJECT LINKS',
		projectTitle: 'Project links',
		projectIntro: 'Agents identify projects; you confirm ownership and permissions.',
		howItWorks: 'How it works',
		singleStep: 'One step',
		promptReady: 'Prompt ready',
		waitingForYou: 'Waiting for you',
		archiveTasks: 'Archive tasks',
		registeredAssets: 'Registered projects',
		runtimeEyebrow: 'BRAIN2 / AGENT CONNECTION',
		runtimeTitle: 'Agent connection',
		runtimeIntro: 'Detect and connect a local agent, then generate a controlled Prompt for the selected work entry.',
		workEntry: 'Work entry',
		detected: 'Detected',
		connectorInstalled: 'Connector installed',
		memoryVerified: 'Current memory verified',
		promptAvailable: 'Prompt always available',
		healthEyebrow: 'BRAIN2 / CHECKS & MAINTENANCE',
		healthTitle: 'Checks and maintenance',
		healthIntro: 'Review local connections, project records, and content status.',
		settingsEyebrow: 'BRAIN2 / SETTINGS',
		settingsTitle: 'Settings',
		settingsIntro: 'Manage devices, memory workspaces, agent access, diagnostics, and updates.',
		deviceManagement: 'Device management',
		currentMemory: 'Current memory',
		multiMemory: 'Additional memory · Advanced',
		obsidian: 'Obsidian',
		agentAccess: 'Agent access',
		diagnostics: 'Diagnostics',
	},
} as const

type CopyKey = keyof typeof copy.zh
const t = (key: CopyKey) => copy[locale.value][key]

const products = [
  { id: 'brain2', name: 'Brain2', code: 'BRAIN2 MANAGER', status: '可用', description: '让 AI 能读懂、接手并持续维护工作上下文的本地外接大脑控制系统。' },
]

const emptyState = (): LauncherState => ({
  brain2_root: '',
  workspace: false,
  project_count: 0,
  agent_count: 0,
  runtime_ready: false,
  health_status: 'not_checked',
  health_score: 0,
  projects: [],
  agents: [],
  launcher_build: 'v0.6.6 WIP',
  launcher_profile: '',
  obsidian_ready: false,
  obsidian_path: '',
  acceptance_mode: '',
  project_candidates: [],
  protector_available: false,
  helper_available: false,
	local_device_id: '',
	selected_device_id: '',
	selected_device_local: false,
	device_setup_required: false,
	local_device_source: '',
	devices: [],
})

const welcome = ref(false)
const selectedProduct = ref('brain2')
const activeView = ref<ViewName>('product')
const state = ref<LauncherState>(emptyState())
const health = ref<HealthReport | null>(null)
const busy = ref(false)
const statusText = ref('READY')
const obsidianDetectionAttempted = ref(false)
const scanAgentID = ref('codex')
const scanRootPath = ref('')
const projectScanResult = ref<ProjectScanResult | null>(null)
const candidateParentSelections = ref<Record<string, string>>({})
const batchCandidateSelections = ref<Record<string, boolean>>({})
const promptModal = ref<PromptModalState | null>(null)
const brainAttachConfirmation = ref<BrainAttachConfirmationState | null>(null)
const promptIntent = ref<'brain' | 'existing_project' | 'new_project'>('brain')
const promptProjectID = ref('')
const detections = ref<AgentDetection[]>([])
const updateDetections = ref<AgentDetection[]>([])
const updateCheckStatus = ref<'idle' | 'checking' | 'checked' | 'failed'>('idle')
const updateCheckedAt = ref('')
const applicationUpdate = ref<ApplicationUpdateResult>({ status: 'not_checked', message: '尚未检查应用更新', current_version: '0.6.6 WIP', checked_at: '' })
const openingLauncherUpdate = ref(false)
const connectionProgress = ref<(AgentProgress & { agent_name: string }) | null>(null)
const helperAuthorizedProjectIDs = ref<string[]>([])
const productLinkProjectID = ref('')
const stateLoadFailed = ref(false)
const elapsedSeconds = ref(0)
let progressTimer: number | undefined
let projectPollTimer: number | undefined
let stopProgressEvents: (() => void) | undefined

const viewCopy = computed(() => {
	const labels: Record<Locale, Record<ViewName, { title: string; context: string }>> = {
		zh: {
			product: { title: '开始使用', context: '产品' },
			projects: { title: '项目连接', context: '项目' },
			runtime: { title: 'Agent 连接', context: '运行' },
			health: { title: '检查与维护', context: '检查' },
			settings: { title: '设置', context: '本机' },
		},
		en: {
			product: { title: 'Getting started', context: 'Product' },
			projects: { title: 'Project links', context: 'Projects' },
			runtime: { title: 'Agent connection', context: 'Runtime' },
			health: { title: 'Checks & maintenance', context: 'Checks' },
			settings: { title: 'Settings', context: 'Local' },
		},
	}
	return labels[locale.value][activeView.value]
})

const progressSteps = computed(() => ['准备环境', '安装接入包', '本地工具检查', '注册 MCP'])
const progressStepIndex = computed(() => {
  const phase = connectionProgress.value?.phase ?? ''
  const localIndexes: Record<string, number> = {
    detecting: 0, installing_host: 1, installing_skill: 1, packaging: 1,
    checking_local: 2, registering_mcp: 3, registered: 4, failed: -1,
  }
  return localIndexes[phase] ?? 0
})
const elapsedLabel = computed(() => {
  const minutes = Math.floor(elapsedSeconds.value / 60)
  const seconds = elapsedSeconds.value % 60
  return minutes > 0 ? `${minutes}:${seconds.toString().padStart(2, '0')}` : `${seconds} 秒`
})

const product = computed(() => products.find((item) => item.id === selectedProduct.value) ?? products[0])
const productLinkProject = computed(() => state.value.projects.find((item) => item.id === productLinkProjectID.value) ?? null)
const productLinkOptions = computed(() => {
  const project = productLinkProject.value
  return [
    {
      id: 'helper' as LinkedProductID,
      name: 'Dev Helper',
      description: locale.value === 'zh' ? '为当前项目签发本次受控授权并打开 Helper。' : 'Issue a controlled grant for this project and open Helper.',
      available: !!project?.helper_eligible && state.value.helper_available,
      status: helperAuthorizedProjectIDs.value.includes(project?.id ?? '')
        ? (locale.value === 'zh' ? '本次已授权' : 'Authorized this session')
        : state.value.helper_available && project?.helper_eligible
          ? (locale.value === 'zh' ? '可打开' : 'Available')
          : (locale.value === 'zh' ? '当前不可用' : 'Unavailable'),
    },
    {
      id: 'protector' as LinkedProductID,
      name: 'Meow Protector',
      description: locale.value === 'zh' ? '打开 Protector，再从 Brain2 已确认项目中选择。' : 'Open Protector and choose from Brain2-confirmed projects.',
      available: !!project?.protector_eligible && state.value.protector_available,
      status: project?.protector_eligible && state.value.protector_available
        ? (locale.value === 'zh' ? '可打开' : 'Available')
        : (locale.value === 'zh' ? '当前不可用' : 'Unavailable'),
    },
    {
      id: 'blueprint' as LinkedProductID,
      name: 'Meow Blueprint',
      description: locale.value === 'zh' ? '产品入口尚未接入 Manager，后续会在这里统一出现。' : 'This product entry is not connected to Manager yet.',
      available: false,
      status: locale.value === 'zh' ? '尚未接入' : 'Not connected',
    },
  ]
})
const isBrain2 = computed(() => selectedProduct.value === 'brain2')
const workspaceLabel = computed(() => state.value.workspace ? '已接入' : '未接入')
const runtimeLabel = computed(() => {
  const verified = state.value.agents.filter((item) => item.status === 'connected').length
  const registered = state.value.agents.filter((item) => item.status === 'registered').length
  if (verified > 0) return `${verified} 个 Agent 已验证`
  if (registered > 0) return `${registered} 个本地已接入`
  return '尚未接入'
})
const healthAttentionCount = computed(() => health.value?.checks.filter((item) => item.status === 'warning' || item.status === 'error').length ?? 0)
const healthCandidateCount = computed(() => health.value?.checks.filter((item) => item.status === 'info').length ?? 0)
const pendingProjectCandidates = computed(() => state.value.project_candidates.filter((item) => item.status === 'pending'))
const selectedProjectCandidates = computed(() => state.value.project_candidates.filter((item) => item.status === 'selected_for_archiving'))
const ignoredProjectCandidates = computed(() => state.value.project_candidates.filter((item) => item.status === 'ignored'))
const archivablePendingCandidates = computed(() => pendingProjectCandidates.value.filter((item) => isArchivableCandidate(item)))
const batchSelectedCandidateIDs = computed(() => archivablePendingCandidates.value.filter((item) => batchCandidateSelections.value[item.id]).map((item) => item.id))
const allArchivableCandidatesSelected = computed(() => archivablePendingCandidates.value.length > 0 && batchSelectedCandidateIDs.value.length === archivablePendingCandidates.value.length)
const scanAgents = computed(() => detections.value.filter((item) => item.detected && item.prompt_connect && hasLocalConnection(item.id)))
const connectorUpdates = computed(() => updateDetections.value.filter((item) => item.mcp_status === 'update_available'))
const ignoredConnectorUpdates = computed(() => updateDetections.value.filter((item) => item.mcp_status === 'update_ignored'))
const updateSummaryLabel = computed(() => {
  if (updateCheckStatus.value === 'idle') return '尚未检查'
  if (updateCheckStatus.value === 'checking') return '正在检查'
  if (updateCheckStatus.value === 'failed') return '检查失败'
  if (connectorUpdates.value.length) return `${connectorUpdates.value.length} 个可更新`
  if (ignoredConnectorUpdates.value.length) return `${ignoredConnectorUpdates.value.length} 个已忽略`
  return '当前已是最新'
})
const updateSummaryHint = computed(() => updateCheckedAt.value ? `上次检查：${updateCheckedAt.value}` : '点击“检查更新”后才读取本机连接器版本')
const updateActionLabel = computed(() => {
  if (updateCheckStatus.value === 'checking') return locale.value === 'zh' ? '检查中…' : 'Checking…'
  if (updateCheckStatus.value === 'failed') return locale.value === 'zh' ? '重新检查' : 'Check again'
  if (updateCheckStatus.value === 'checked') return locale.value === 'zh' ? '重新检查' : 'Check again'
  return locale.value === 'zh' ? '检查更新' : 'Check for updates'
})
const applicationUpdateLabel = computed(() => {
  if (applicationUpdate.value.status === 'update_available') return locale.value === 'zh' ? '发现新版本' : 'Update available'
  if (applicationUpdate.value.status === 'current') return locale.value === 'zh' ? '已是最新' : 'Up to date'
  if (applicationUpdate.value.status === 'not_checked') return locale.value === 'zh' ? '正在检查' : 'Checking'
  return locale.value === 'zh' ? '暂时无法检查' : 'Check unavailable'
})
const applicationUpdateActionLabel = computed(() => applicationUpdate.value.status === 'update_available'
  ? openingLauncherUpdate.value
    ? (locale.value === 'zh' ? '正在打开 Launcher…' : 'Opening Launcher…')
    : (locale.value === 'zh' ? '用 Launcher 更新' : 'Update in Launcher')
  : (locale.value === 'zh' ? '重新检查' : 'Check again'))
const settingsUpdateSummaryLabel = computed(() => {
  if (updateCheckStatus.value === 'checking') return locale.value === 'zh' ? '正在检查应用与连接组件' : 'Checking the app and connectors'
  const available = Number(applicationUpdate.value.status === 'update_available') + connectorUpdates.value.length
  if (available) return locale.value === 'zh' ? `${available} 项更新可用` : `${available} update${available === 1 ? '' : 's'} available`
  if (applicationUpdate.value.status === 'check_failed' || updateCheckStatus.value === 'failed') return locale.value === 'zh' ? '部分状态暂时无法检查' : 'Some update status is unavailable'
  if (applicationUpdate.value.status === 'current' && updateCheckStatus.value === 'checked') return locale.value === 'zh' ? '当前已是最新' : 'Everything is up to date'
  return locale.value === 'zh' ? '检查 Manager 与 Brain2 连接组件' : 'Check Manager and Brain2 connectors'
})
const applicationUpdateVersionLabel = computed(() => applicationUpdate.value.release
  ? `${applicationUpdate.value.current_version} → ${applicationUpdate.value.release.version}`
  : applicationUpdate.value.current_version)
const missingDeviceID = computed(() => ['A', 'B'].find((id) => !state.value.devices.some((device) => device.id === id)) ?? '')
const localDeviceIdentity = computed(() => {
	const localDevice = state.value.devices.find((device) => device.local || device.id === state.value.local_device_id)
	return {
		avatar: state.value.local_device_id || '·',
		name: localDevice?.machine_name || (state.value.local_device_id ? `设备 ${state.value.local_device_id}` : '本机身份未登记'),
		meta: state.value.local_device_id ? `${t('localIdentity')} · ${state.value.local_device_id}` : t('waitingIdentity'),
	}
})
const localDeviceSourceLabel = computed(() => {
	const labels: Record<string, string> = {
		'local_setting+machine_name': '本机设置与 Windows 设备名一致',
		'local_setting+brain2_path': '本机设置与 Brain2 路径一致',
		local_setting: '由这台电脑的 Launcher 本地设置确认',
		machine_name: '按 Windows 设备名自动识别',
		brain2_path: '按本机 Brain2 路径自动识别',
		brain2_path_convention: '按 A/B 约定路径自动识别',
		first_device: '当前是这套 Brain2 登记的第一台设备',
	}
	return labels[state.value.local_device_source] ?? '等待确认本机设备'
})
const healthLabel = computed(() => {
  if (!health.value) return '尚未检查'
  if (health.value.overall_status === 'healthy') return healthCandidateCount.value ? `可用 · ${healthCandidateCount.value} 个候选项` : '可用'
  if (health.value.overall_status === 'critical') return `部分功能不可用 · ${healthAttentionCount.value} 项`
  return `可用 · ${healthAttentionCount.value} 项需要关注`
})
const homeStage = computed<{ id: HomeStageID; step: string; title: string; description: string }>(() => {
  if (!state.value.workspace) {
    return {
      id: 'memory',
      step: '01 / 04',
      title: locale.value === 'zh' ? '连接协同记忆库' : 'Connect a memory workspace',
      description: locale.value === 'zh'
        ? '选择已有 Obsidian 仓库，或在空文件夹中创建新的 Brain2。现有内容不会被迁移或整理。'
        : 'Choose an existing Obsidian workspace or create a new Brain2 in an empty folder. Existing content stays untouched.',
    }
  }
  if (!state.value.obsidian_ready) {
    return {
      id: 'obsidian',
      step: '02 / 04',
      title: locale.value === 'zh' ? '准备 Obsidian' : 'Prepare Obsidian',
      description: locale.value === 'zh'
        ? '先自动检测本机安装。未找到时，再交给 Agent 安装或手动定位。'
        : 'Detect the local installation first. If it is not found, ask an agent to install it or locate it manually.',
    }
  }
  if (!state.value.runtime_ready) {
    return {
      id: 'agent',
      step: '03 / 04',
      title: locale.value === 'zh' ? '连接 Agent' : 'Connect an agent',
      description: locale.value === 'zh'
        ? '让 Agent 读取 Brain2 上下文，并在权限边界内执行、交接和回填成果。'
        : 'Let an agent read Brain2 context and work, hand off, and backfill results within governed permissions.',
    }
  }
  if (state.value.project_count === 0) {
    return {
      id: 'project',
      step: '04 / 04',
      title: locale.value === 'zh' ? '识别第一个项目' : 'Discover the first project',
      description: locale.value === 'zh'
        ? '由 Agent 根据 Brain2、交接记录和本机证据识别项目；你只确认归属与授权。'
        : 'The agent identifies projects from Brain2, handoffs, and local evidence. You only confirm ownership and permission.',
    }
  }
  return {
    id: 'ready',
    step: locale.value === 'zh' ? '设置完成' : 'Setup complete',
    title: locale.value === 'zh' ? 'Brain2 已准备好' : 'Brain2 is ready',
    description: locale.value === 'zh'
      ? '记忆库、Obsidian 和 Agent 均已连接。你可以继续项目工作，或检查当前状态。'
      : 'The memory workspace, Obsidian, and an agent are connected. Continue project work or review the current status.',
  }
})
const homeSteps = computed(() => [
  {
    id: 'memory',
    number: '01',
    label: locale.value === 'zh' ? '连接记忆库' : 'Connect memory',
    detail: state.value.workspace
      ? (locale.value === 'zh' ? '已连接' : 'Connected')
      : (locale.value === 'zh' ? '等待选择' : 'Waiting'),
    complete: state.value.workspace,
    current: homeStage.value.id === 'memory',
  },
  {
    id: 'obsidian',
    number: '02',
    label: locale.value === 'zh' ? '准备 Obsidian' : 'Prepare Obsidian',
    detail: state.value.obsidian_ready
      ? (locale.value === 'zh' ? '已检测' : 'Detected')
      : (locale.value === 'zh' ? '尚未检测' : 'Not detected'),
    complete: state.value.obsidian_ready,
    current: homeStage.value.id === 'obsidian',
  },
  {
    id: 'agent',
    number: '03',
    label: locale.value === 'zh' ? '连接 Agent' : 'Connect agent',
    detail: state.value.runtime_ready
      ? (locale.value === 'zh' ? '已验证' : 'Verified')
      : (locale.value === 'zh' ? '等待连接' : 'Waiting'),
    complete: state.value.runtime_ready,
    current: homeStage.value.id === 'agent',
  },
  {
    id: 'project',
    number: '04',
    label: locale.value === 'zh' ? '连接项目' : 'Link projects',
    detail: state.value.project_count > 0
      ? (locale.value === 'zh' ? `${state.value.project_count} 个已登记` : `${state.value.project_count} registered`)
      : (locale.value === 'zh' ? '尚未登记' : 'None registered'),
    complete: state.value.project_count > 0,
    current: homeStage.value.id === 'project',
  },
])
const homeCompletedSteps = computed(() => homeSteps.value.filter((item) => item.complete).length)
const healthCategoryLabels: Record<string, string> = {
  connection_core: '连接与核心结构',
  project_registry: '项目登记与路径',
  content_graph: '内容链接与孤立候选',
  broken_link: '断链',
  missing_attachment: '附件缺失',
  ambiguous_short_link: '短链歧义',
  orphan_candidate: '孤立候选',
  report: '报告状态',
}
function healthCategoryLabel(category: string) {
  return healthCategoryLabels[category] ?? category
}

function setLocale(next: Locale) {
	locale.value = next
	window.localStorage.setItem('meow-launcher-locale', next)
	document.documentElement.lang = next === 'zh' ? 'zh-CN' : 'en'
	statusText.value = ''
}

function agentIcon(agentID: string) {
  if (agentID === 'hermes') return hermesIcon
  if (agentID.includes('claude')) return '/assets/claude.svg'
  if (agentID.includes('codex')) return '/assets/openai.svg'
  return '/assets/meow-forge-symbol-reverse-hd.png'
}

function setView(view: ViewName) {
  activeView.value = view
  if (view === 'runtime') {
    void (async () => {
      await refreshAgents()
    })()
  }
  if (view === 'projects') {
    void (async () => {
      await refreshState()
      await refreshAgents()
    })()
  }
}

function changeProduct() {
  activeView.value = 'product'
  statusText.value = `${product.value.name.toUpperCase()} SELECTED`
}

async function refreshState() {
  try {
    state.value = await backend.state()
    if (stateLoadFailed.value) statusText.value = state.value.workspace ? 'BRAIN2 CONNECTED' : 'READY'
    stateLoadFailed.value = false
  } catch (error) {
    stateLoadFailed.value = true
    statusText.value = errorMessage(error)
  }
}

async function changeDevice(event: Event) {
	const select = event.target as HTMLSelectElement
	const deviceID = select.value
	if (!deviceID) return
	busy.value = true
	try {
		state.value = await backend.switchDevice(deviceID)
		statusText.value = state.value.selected_device_local
			? `已切换到设备 ${deviceID}（本机）`
			: `正在查看设备 ${deviceID} 的远程记录`
		health.value = null
	} catch (error) {
		statusText.value = errorMessage(error)
		select.value = state.value.selected_device_id
	} finally {
		busy.value = false
	}
}

async function setLocalDevice(deviceID: string) {
	if (busy.value) return
	busy.value = true
	try {
		state.value = await backend.setLocalDevice(deviceID)
		statusText.value = `这台电脑已登记为设备 ${deviceID}`
	} catch (error) {
		statusText.value = errorMessage(error)
	} finally {
		busy.value = false
	}
}

async function addMissingDevice() {
	const deviceID = missingDeviceID.value
	if (busy.value || !deviceID) return
	busy.value = true
	try {
		state.value = await backend.addDevice(deviceID)
		statusText.value = `设备 ${deviceID} 已加入当前 Brain2；可从下拉框查看`
	} catch (error) {
		statusText.value = errorMessage(error)
	} finally {
		busy.value = false
	}
}

async function removeManagedDevice(deviceID: string) {
	if (busy.value || deviceID === state.value.local_device_id) return
	const confirmed = window.confirm(`确认把设备 ${deviceID} 移出当前管理列表？\n\n这不会删除该设备的 Agent 历史记录；以后重新添加设备 ${deviceID} 时，历史记录仍可恢复显示。`)
	if (!confirmed) return
	busy.value = true
	try {
		state.value = await backend.removeDevice(deviceID)
		statusText.value = `设备 ${deviceID} 已移出管理列表；历史记录未删除`
	} catch (error) {
		statusText.value = errorMessage(error)
	} finally {
		busy.value = false
	}
}

async function switchToLocalDevice() {
	if (!state.value.local_device_id) return
	busy.value = true
	try {
		state.value = await backend.switchDevice(state.value.local_device_id)
		statusText.value = `已切回设备 ${state.value.local_device_id}（本机）`
	} catch (error) {
		statusText.value = errorMessage(error)
	} finally {
		busy.value = false
	}
}

async function refreshAgents() {
  try {
    detections.value = await backend.detectAgents()
    if (!scanAgents.value.some((item) => item.id === scanAgentID.value)) {
      scanAgentID.value = scanAgents.value[0]?.id ?? ''
    }
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function showPromptModal(title: string, subtitle: string, prompt: string, eyebrow = 'PROMPT READY', updateSummary?: UpdateSummary) {
  promptModal.value = { eyebrow, title, subtitle, prompt, copied: false, updateSummary }
  if (!prompt) return
  try {
    await backend.copyText(prompt)
    if (promptModal.value?.prompt === prompt) promptModal.value.copied = true
    statusText.value = 'Prompt 已复制；请粘贴给 Agent'
  } catch {
    statusText.value = 'Prompt 已显示；自动复制失败，可在弹窗中重新复制'
  }
}

async function copyActivePrompt() {
  const current = promptModal.value
  if (!current) return
  try {
    await backend.copyText(current.prompt)
    if (promptModal.value?.prompt === current.prompt) promptModal.value.copied = true
    statusText.value = 'Prompt 已复制；请粘贴给 Agent'
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

function closePromptModal() {
  promptModal.value = null
}

async function prepareProjectScan() {
  try {
    if (!scanAgents.value.some((item) => item.id === scanAgentID.value)) {
      statusText.value = '请先在 Agent 连接页完成一个本地接入'
      return
    }
    const result = await backend.prepareProjectScan(scanAgentID.value, scanRootPath.value)
    projectScanResult.value = result
    statusText.value = result.message
    await showPromptModal('把这段项目发现任务交给 Agent', result.scan_root ? `兜底扫描范围：${result.scan_root}` : '从 Brain2、GitHub 与本机证据寻找', result.prompt, 'PROJECT DISCOVERY READY')
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function prepareProjectArchive(projectID: string) {
  const agentID = scanAgentID.value || scanAgents.value[0]?.id
  if (busy.value || !agentID) return
  busy.value = true
  try {
    const result = await backend.prepareProjectArchivePrompt(agentID, projectID)
    statusText.value = result.message
    const projectName = state.value.projects.find((item) => item.id === projectID)?.name || '当前项目'
    await showPromptModal('让 Agent 提议项目档案草稿', `项目：${projectName}`, result.prompt, 'ARCHIVE DRAFT READY')
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function chooseProjectScanRoot() {
  try {
    const selected = await backend.chooseDirectory('选择项目可能所在的父文件夹（不是 Brain2 文件夹）')
    if (!selected) return
    scanRootPath.value = selected
    projectScanResult.value = null
    statusText.value = '扫描范围已选择；确认 Agent 后生成提示词'
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function copyProjectScanPrompt() {
  if (!projectScanResult.value) return
  await showPromptModal('把这段项目发现任务交给 Agent', projectScanResult.value.scan_root ? `兜底扫描范围：${projectScanResult.value.scan_root}` : '从 Brain2、GitHub 与本机证据寻找', projectScanResult.value.prompt, 'PROJECT DISCOVERY READY')
}

function snapshotUpdateResults() {
  updateDetections.value = detections.value.map((item) => ({ ...item }))
  updateCheckStatus.value = 'checked'
  updateCheckedAt.value = new Date().toLocaleString('zh-CN', { hour12: false })
}

async function confirmProjectCandidate(candidateID: string) {
  const agentID = scanAgentID.value || scanAgents.value[0]?.id
  if (busy.value || !agentID) {
    statusText.value = '请先连接一个 Agent'
    return
  }
  busy.value = true
  try {
    const result = await backend.prepareProjectCandidateArchive(agentID, candidateID)
    state.value = await backend.state()
    statusText.value = result.message
    await showPromptModal('把这段专属建档任务交给 Agent', `项目候选：${result.project_name}`, result.prompt, 'PROJECT ARCHIVE READY')
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

function toggleBatchCandidate(candidateID: string) {
  if (!isArchivableCandidate(state.value.project_candidates.find((item) => item.id === candidateID))) return
  batchCandidateSelections.value = { ...batchCandidateSelections.value, [candidateID]: !batchCandidateSelections.value[candidateID] }
}

function toggleAllArchivableCandidates() {
  const next = !allArchivableCandidatesSelected.value
  const selections = { ...batchCandidateSelections.value }
  for (const candidate of archivablePendingCandidates.value) selections[candidate.id] = next
  batchCandidateSelections.value = selections
}

async function prepareBatchProjectArchives() {
  const agentID = scanAgentID.value || scanAgents.value[0]?.id
  const candidateIDs = batchSelectedCandidateIDs.value
  if (busy.value || !agentID) {
    statusText.value = '请先连接一个 Agent'
    return
  }
  if (!candidateIDs.length) {
    statusText.value = '请先选择至少一个可建档候选'
    return
  }
  busy.value = true
  try {
    const result = await backend.prepareProjectCandidateArchives(agentID, candidateIDs)
    state.value = await backend.state()
    batchCandidateSelections.value = {}
    statusText.value = result.message
    await showPromptModal('把这段批量建档任务交给 Agent', `已选择 ${result.items.length} 个项目；Agent 将逐项目调查并回填回执`, result.prompt, 'BATCH PROJECT ARCHIVE READY')
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function ignoreProjectCandidate(candidateID: string) {
  await runStateAction('忽略项目候选', () => backend.ignoreProjectCandidate(candidateID))
}

async function assignProjectCandidate(candidateID: string) {
  const parentProjectID = candidateParentSelections.value[candidateID]
  if (!parentProjectID) {
    statusText.value = '请先选择它归属的已有项目'
    return
  }
  await runStateAction('归入已有项目', () => backend.assignProjectCandidateToParent(candidateID, parentProjectID))
}

function isArchivableCandidate(candidate?: LauncherState['project_candidates'][number]) {
  return !!candidate && ['code_project', 'application', 'product', 'needs_verification'].includes(candidate.detected_kind)
}

async function openProtectorProjectPicker() {
  if (busy.value) return
  busy.value = true
  statusText.value = '正在把 Brain2 已确认项目清单交给 Protector...'
  try {
    const result = await backend.openProtectorProjectPicker()
    statusText.value = result.message
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function openDevHelper(projectID: string) {
  if (busy.value) return
  busy.value = true
  statusText.value = '正在为当前项目签发 Dev Helper 本次授权...'
  try {
    const result = await backend.openDevHelper(projectID)
    if (!helperAuthorizedProjectIDs.value.includes(result.project_id)) {
      helperAuthorizedProjectIDs.value = [...helperAuthorizedProjectIDs.value, result.project_id]
    }
    statusText.value = result.message
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

function openProductLinkMenu(projectID: string) {
  productLinkProjectID.value = projectID
}

function closeProductLinkMenu() {
  productLinkProjectID.value = ''
}

async function openLinkedProduct(productID: LinkedProductID) {
  const project = productLinkProject.value
  if (!project || busy.value) return
  closeProductLinkMenu()
  if (productID === 'helper') {
    await openDevHelper(project.id)
    return
  }
  if (productID === 'protector') {
    await openProtectorProjectPicker()
  }
}

function connectionFor(agentID: string) {
  return state.value.agents.find((item) => item.id === agentID)
}

function connectionLabel(agentID: string) {
  const status = connectionFor(agentID)?.status
  const labels: Record<Locale, Record<string, string>> = {
    zh: { prepared: '已准备', registered: '本地已接入', awaiting_handshake: '验证中', connected: 'Agent 已验证', failed: '接入失败' },
    en: { prepared: 'Prepared', registered: 'Registered locally', awaiting_handshake: 'Verifying', connected: 'Agent verified', failed: 'Failed' },
  }
  return labels[locale.value][status ?? ''] ?? (locale.value === 'zh' ? '未连接' : 'Not connected')
}

function agentMessage(agent: AgentDetection) {
  if (!state.value.selected_device_local) {
    return locale.value === 'zh'
      ? '远程设备不执行本机环境检测；这里只显示同步到 Brain2 的历史接入状态。'
      : 'Remote devices do not run local detection. This view only shows connection history synced to Brain2.'
  }
  if (locale.value === 'zh') return agent.message
  if (!agent.detected) return 'No supported client detected'
  if (agent.id === 'codex-desktop') return 'Codex Desktop detected · user configuration can be updated safely'
  if (agent.id === 'hermes') return 'Quick connection available · WSL Ubuntu-Hermes'
  return 'Quick connection available'
}

async function runStateAction(label: string, action: () => Promise<LauncherState>) {
  if (busy.value) return
  busy.value = true
  statusText.value = `${label}...`
  try {
    state.value = await action()
    statusText.value = `${label}完成`
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function chooseAndInstall() {
  try {
    const root = await backend.chooseDirectory('选择一个空文件夹，用于创建新的协同记忆仓库')
    if (root) await runStateAction('创建协同记忆仓库', () => backend.installBrain2(root))
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function chooseAndAttachBrain2() {
  try {
    const root = await backend.chooseDirectory('选择已有的 Obsidian 仓库（只新增 Brain2 连接层，不整理内容）')
    if (root) brainAttachConfirmation.value = { root }
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function confirmAttachBrain2() {
  const pending = brainAttachConfirmation.value
  if (!pending || busy.value) return
  busy.value = true
  statusText.value = '正在增量接入已有 Obsidian 仓库...'
  try {
    state.value = await backend.attachBrain2(pending.root)
    brainAttachConfirmation.value = null
    statusText.value = '已有 Obsidian 仓库已增量接入'
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function attachExternalProject() {
  try {
    const source = await backend.chooseDirectory('选择外部项目目录（不会移动源文件）')
    if (!source) return
    await runStateAction('连接项目目录', () => backend.attachExternalProject('', source, ''))
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

function startNewProjectWithAgent() {
  promptIntent.value = 'new_project'
  setView('runtime')
  statusText.value = '请选择一个 Agent；它会先与你确认目标，再按 Brain2 规则建立项目'
}

async function prepareObsidianInstall() {
  try {
    const target = await backend.chooseDirectory('选择希望安装 Obsidian 的位置')
    if (!target) return
    const prompt = await backend.prepareObsidianInstallPrompt(target)
    await showPromptModal('把这段安装任务交给 Agent', `期望安装位置：${target}`, prompt, 'OB INSTALL READY')
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function detectObsidianInstallation() {
  if (busy.value) return
  busy.value = true
  statusText.value = locale.value === 'zh' ? '正在检测 Obsidian…' : 'Detecting Obsidian…'
  try {
    const detectedState = await backend.detectObsidian()
    state.value = detectedState
    obsidianDetectionAttempted.value = true
    statusText.value = detectedState.obsidian_ready
      ? (locale.value === 'zh' ? `已检测到 Obsidian：${detectedState.obsidian_path}` : `Obsidian detected: ${detectedState.obsidian_path}`)
      : (locale.value === 'zh' ? '未检测到 Obsidian；可以交给 Agent 安装，或手动定位。' : 'Obsidian was not detected. Ask an agent to install it, or locate it manually.')
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function chooseObsidianExecutable() {
  await runStateAction('选择已安装的 Obsidian', backend.chooseObsidianExecutable)
}

async function confirmProjectManifest(item: LauncherState['projects'][number]) {
  let sourcePath = ''
  if (item.mode === 'external') {
    try {
      sourcePath = await backend.chooseDirectory(item.manifest_path ? '重新定位外部项目目录' : '选择并确认外部项目目录')
    } catch (error) {
      statusText.value = errorMessage(error)
      return
    }
    if (!sourcePath) return
  }
  const label = item.manifest_path ? '重新定位项目' : '补登记项目身份'
  await runStateAction(label, () => backend.confirmProjectManifest(item.id, sourcePath))
}

async function quickConnect(agent: AgentDetection, replaceExistingRoot = false) {
  if (busy.value) return
  busy.value = true
  startConnectionProgress(agent)
  statusText.value = `正在本地接入 ${agent.name}...`
  try {
    const result = replaceExistingRoot ? await backend.rebindAgent(agent.id) : await backend.quickConnectAgent(agent.id)
    statusText.value = result.message
    await refreshState()
    await refreshAgents()
    snapshotUpdateResults()
    if (result.skill_conflict) {
      await showPromptModal(
        `${agent.name} 更新已暂停`,
        '检测到用户修改过的 Skill。Launcher 没有覆盖它，也没有继续改写连接器配置。',
        '',
        'UPDATE CONFLICT',
        { skill: '存在本地修改，未覆盖', host: '未继续安装', config: '未修改', verification: '解决冲突后再更新', conflict: true },
      )
    } else if (result.verification_prompt || result.prompt) {
      const prompt = result.verification_prompt || result.prompt
      await showPromptModal(
        result.restart_required ? `Brain2 连接器已安装，请重启 ${agent.name}` : `Brain2 连接器检查完成`,
        result.restart_required ? '安装已完成，但当前会话不会热加载新版 Skill。完全退出并重开 Agent 后，把下面的检查 Prompt 交给它。' : '本机文件与配置已经是当前受控版本；可以直接使用下面的 Prompt 重新验证。',
        prompt,
        result.restart_required ? 'UPDATE INSTALLED · RESTART REQUIRED' : 'CONNECTOR CURRENT · VERIFY READY',
        {
          skill: result.skill_updated ? '已备份并安装新版' : '已是 Launcher 管理版本',
          host: result.host_updated ? `已安装 v${agent.mcp_available_version}` : `v${agent.mcp_available_version} 已存在`,
          config: result.config_updated ? '已备份并更新' : '无需变更',
          verification: result.restart_required ? '等待重启后握手验证' : '无需重启',
        },
      )
    }
  } catch (error) {
    statusText.value = errorMessage(error)
    await refreshState()
  } finally {
    await new Promise((resolve) => window.setTimeout(resolve, connectionProgress.value?.phase === 'connected' ? 650 : 250))
    stopConnectionProgress()
    busy.value = false
  }
}

function hasLocalConnection(agentID: string) {
  const status = connectionFor(agentID)?.status
  return status === 'registered' || status === 'connected'
}

function agentPrimaryLabel(agent: AgentDetection) {
  if (agent.id === 'claude-desktop') return locale.value === 'zh' ? '准备扩展' : 'Prepare extension'
  if (agent.mcp_status === 'conflict') return locale.value === 'zh' ? '改用当前 Launcher' : 'Use this Launcher'
  if (agent.mcp_status === 'update_available') return locale.value === 'zh' ? '更新连接器' : 'Update connector'
  if (hasLocalConnection(agent.id)) return locale.value === 'zh' ? '重新安装' : 'Reinstall'
  return locale.value === 'zh' ? '快速接入' : 'Quick connect'
}

function runAgentPrimary(agent: AgentDetection) {
  if (agent.mcp_status === 'conflict') {
    const confirmed = window.confirm(`确认让 ${agent.name} 改用当前 Launcher 的记忆库注册表？\n\n这只替换该 Agent 的 brain2-meow 连接器配置，不修改任何记忆库内容。`)
    if (!confirmed) return
    return quickConnect(agent, true)
  }
  return quickConnect(agent)
}

async function ignoreConnectorUpdate(agent: AgentDetection) {
  if (busy.value) return
  busy.value = true
  try {
    await backend.ignoreConnectorUpdate(agent.id)
    await refreshAgents()
    snapshotUpdateResults()
    statusText.value = `已忽略 ${agent.name} 的连接器 v${agent.mcp_available_version} 提醒`
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function restoreConnectorUpdate(agent: AgentDetection) {
  if (busy.value) return
  busy.value = true
  try {
    await backend.restoreConnectorUpdate(agent.id)
    await refreshAgents()
    snapshotUpdateResults()
    statusText.value = `已恢复 ${agent.name} 的连接器更新提醒`
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function rollbackConnector(agent: AgentDetection) {
  if (busy.value || !agent.rollback_available) return
  busy.value = true
  try {
    const result = await backend.rollbackAgentConnector(agent.id)
    statusText.value = result.message
    await refreshAgents()
    snapshotUpdateResults()
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function updateAllConnectors() {
  if (busy.value || !connectorUpdates.value.length) return
  const pending = [...connectorUpdates.value]
  for (const agent of pending) await quickConnect(agent)
  statusText.value = '所选 Brain2 连接器已更新'
}

async function runSettingsUpdateAction() {
  if (busy.value) return
  busy.value = true
  updateCheckStatus.value = 'checking'
  statusText.value = locale.value === 'zh' ? '正在检查 Manager 与 Brain2 连接组件更新…' : 'Checking Manager and Brain2 connector updates…'
  await checkApplicationUpdate()
  try {
    const results = await backend.detectAgents()
    detections.value = results
    snapshotUpdateResults()
    const available = Number(applicationUpdate.value.status === 'update_available') + connectorUpdates.value.length
    statusText.value = available
      ? (locale.value === 'zh' ? `发现 ${available} 项更新` : `${available} update${available === 1 ? '' : 's'} available`)
      : (locale.value === 'zh' ? '检查完成；当前已是最新版本' : 'Check complete; everything is up to date')
  } catch (error) {
    updateCheckStatus.value = 'failed'
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function checkApplicationUpdate() {
  try {
    applicationUpdate.value = await backend.checkApplicationUpdate()
  } catch (error) {
    applicationUpdate.value = { status: 'check_failed', message: errorMessage(error), current_version: '0.6.6 WIP', checked_at: new Date().toISOString() }
  }
}

async function runApplicationUpdateAction() {
  if (applicationUpdate.value.status !== 'update_available') return checkApplicationUpdate()
  openingLauncherUpdate.value = true
  try {
    await backend.openApplicationUpdate()
    statusText.value = '已打开 Meow Launcher 的 Brain2 Manager 更新详情'
  } catch (error) {
    statusText.value = errorMessage(error)
	openingLauncherUpdate.value = false
	return
  }
  window.setTimeout(() => { openingLauncherUpdate.value = false }, 2500)
}

function startConnectionProgress(agent: AgentDetection) {
  stopConnectionProgress()
  elapsedSeconds.value = 0
  connectionProgress.value = {
    agent_id: agent.id,
    agent_name: agent.name,
    phase: 'detecting',
    title: '正在启动本地快速接入',
    detail: '本阶段只检查本地 Skill、MCP Host 和配置，不调用模型。',
  }
  progressTimer = window.setInterval(() => { elapsedSeconds.value += 1 }, 1000)
}

function stopConnectionProgress() {
  if (progressTimer !== undefined) {
    window.clearInterval(progressTimer)
    progressTimer = undefined
  }
  connectionProgress.value = null
}

function receiveConnectionProgress(progress: AgentProgress) {
  const current = connectionProgress.value
  if (!current || current.agent_id !== progress.agent_id) return
  connectionProgress.value = { ...progress, agent_name: current.agent_name }
  statusText.value = progress.title
}

async function preparePrompt(agent: AgentDetection) {
  if (busy.value) return
  busy.value = true
  statusText.value = `正在生成 ${agent.name} Prompt...`
  try {
    const result = await backend.prepareAgentPrompt(
      agent.id,
      promptIntent.value,
      promptProjectID.value,
      '',
      '',
    )
    statusText.value = result.message
    await refreshState()
    await showPromptModal(`把这段 Prompt 交给 ${agent.name}`, '先检查内容，再粘贴到你实际使用的 Agent 会话。', result.prompt, 'WORK PROMPT READY')
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

function promptButtonLabel(agentID: string) {
  if (hasLocalConnection(agentID)) return locale.value === 'zh' ? '查看开始 Prompt' : 'View start Prompt'
  return locale.value === 'zh' ? '查看完整接入 Prompt' : 'View setup Prompt'
}

async function runHealth() {
  if (busy.value) return
  busy.value = true
  statusText.value = '健康检查中...'
  try {
    health.value = await backend.runHealthCheck()
    state.value.health_status = health.value.overall_status
    state.value.health_score = health.value.score
    statusText.value = `确定性检查完成：${healthLabel.value}`
  } catch (error) {
    statusText.value = errorMessage(error)
  } finally {
    busy.value = false
  }
}

async function prepareHealthPrompt() {
  try {
    const prompt = await backend.prepareHealthAgentPrompt()
    health.value = await backend.runHealthCheck()
    await showPromptModal('把检查证据交给 Agent 分析', `记忆库：${state.value.brain2_root}`, prompt, 'HEALTH REVIEW READY')
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function simpleAction(label: string, action: () => Promise<void>) {
  try {
    await action()
    statusText.value = label
  } catch (error) {
    statusText.value = errorMessage(error)
  }
}

async function enterLocalTrial() {
  welcome.value = false
  await refreshState()
}

onMounted(async () => {
  document.documentElement.lang = locale.value === 'zh' ? 'zh-CN' : 'en'
  stopProgressEvents = EventsOn('agent-connect-progress', receiveConnectionProgress)
  await refreshState()
	await checkApplicationUpdate()
  await refreshAgents()
  projectPollTimer = window.setInterval(() => {
    if (activeView.value === 'projects' && !busy.value) void refreshState()
  }, 3000)
})

onUnmounted(() => {
  stopProgressEvents?.()
  stopConnectionProgress()
  if (projectPollTimer !== undefined) window.clearInterval(projectPollTimer)
})
</script>

<template>
  <div class="window-shell" :class="{ 'welcome-mode': welcome }">
    <header class="titlebar">
      <div class="title-brand">
        <span class="title-dot"></span>
        <span>BRAIN2 MANAGER</span>
        <small>{{ state.launcher_build }}<template v-if="state.launcher_profile"> · {{ state.launcher_profile }}</template></small>
      </div>
      <div class="window-controls">
        <button :aria-label="locale === 'zh' ? '最小化' : 'Minimize'" @click="backend.minimise"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8.5h10" /></svg></button>
        <button :aria-label="locale === 'zh' ? '最大化' : 'Maximize'" @click="backend.maximise"><svg viewBox="0 0 16 16" aria-hidden="true"><rect x="3.5" y="3.5" width="9" height="9" /></svg></button>
        <button class="close" :aria-label="locale === 'zh' ? '关闭' : 'Close'" @click="backend.close"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8" /></svg></button>
      </div>
    </header>

    <section v-if="welcome" class="welcome-screen">
      <div class="welcome-grid"></div>
      <div class="welcome-shell">
        <img class="welcome-logo" src="/assets/meow-forge-symbol-reverse-hd.png" alt="Meow Forge symbol" />
        <div class="kicker">MEOW PRODUCT ENTRY</div>
        <h1>meow launcher</h1>
        <p>先在本机体验 Brain2；正式账户、授权与跨设备服务将在后续版本提供。</p>
        <div class="login-pane local-trial-pane">
          <span>LOCAL-FIRST PREVIEW</span>
          <strong>当前版本无需注册账号</strong>
          <p>产品配置和协同记忆库都保存在你的电脑上。进入后可接入已有 Obsidian 仓库，或创建一套新的协同记忆仓库。</p>
          <button class="primary" @click="enterLocalTrial">进入本地试用</button>
          <p class="login-note">本地试用不代表正式账户授权；后续升级时会明确说明登录与数据边界。</p>
        </div>
      </div>
      <footer class="welcome-status"><span>设备 {{ state.local_device_id || 'A' }}</span><span>等待本机工作区</span></footer>
    </section>

    <section v-else class="app-screen">
      <aside class="sidebar">
        <div class="brand-block">
          <img src="/assets/meow-forge-symbol-reverse-hd.png" alt="Meow symbol" />
          <div><strong>brain2</strong><span>manager</span></div>
        </div>

		<label class="select-label device-label" for="device-select">{{ t('currentDevice') }}</label>
		<select id="device-select" class="product-select device-select" :value="state.selected_device_id" :disabled="busy || !state.workspace || state.device_setup_required" @change="changeDevice">
			<option v-if="!state.devices.length" value="">请先接入 Brain2</option>
			<option v-for="device in state.devices" :key="device.id" :value="device.id">{{ device.id }}{{ device.machine_name ? `（${device.machine_name}）` : '' }} · {{ device.local ? (locale === 'zh' ? '本机' : 'Local') : (locale === 'zh' ? '远程' : 'Remote') }}</option>
		</select>
		<div v-if="state.workspace && !state.device_setup_required" class="device-quick-actions"><button :disabled="busy || !missingDeviceID" @click="addMissingDevice">＋ {{ t('addDevice') }}</button><button @click="setView('settings')">{{ t('manage') }}</button></div>

        <nav class="side-nav">
          <button :class="{ active: activeView === 'product' }" @click="setView('product')"><span>01</span>{{ locale === 'zh' ? '开始使用' : 'Getting started' }}</button>
          <template v-if="isBrain2">
            <button :class="{ active: activeView === 'runtime' }" @click="setView('runtime')"><span>02</span>{{ locale === 'zh' ? 'Agent 连接' : 'Agent connection' }}</button>
            <button :class="{ active: activeView === 'projects' }" @click="setView('projects')"><span>03</span>{{ locale === 'zh' ? '项目连接' : 'Project links' }}</button>
            <button :class="{ active: activeView === 'health' }" @click="setView('health')"><span>04</span>{{ locale === 'zh' ? '检查与维护' : 'Checks' }}</button>
            <button :class="{ active: activeView === 'settings', 'has-notification': applicationUpdate.status === 'update_available' }" :aria-label="applicationUpdate.status === 'update_available' ? (locale === 'zh' ? '设置，有可用更新' : 'Settings, update available') : (locale === 'zh' ? '设置' : 'Settings')" @click="setView('settings')"><span>05</span>{{ locale === 'zh' ? '设置' : 'Settings' }}<i v-if="applicationUpdate.status === 'update_available'" class="nav-notification-dot" aria-hidden="true"></i></button>
          </template>
        </nav>

        <div class="side-foot">
          <span class="user-avatar">{{ localDeviceIdentity.avatar }}</span>
          <div><strong>{{ localDeviceIdentity.name }}</strong><small>{{ localDeviceIdentity.meta }}</small></div>
          <span class="identity-state" :class="{ connected: state.workspace }">{{ state.workspace ? t('connected') : t('waitingConnection') }}</span>
        </div>
      </aside>

      <main class="main-area">
        <div class="toolbar">
          <div><strong>{{ viewCopy.title }}</strong><span>{{ viewCopy.context }}</span></div>
          <div class="toolbar-actions">
            <div class="locale-switch" role="group" :aria-label="locale === 'zh' ? '语言' : 'Language'">
              <button :class="{ active: locale === 'zh' }" @click="setLocale('zh')">中文</button>
              <button :class="{ active: locale === 'en' }" @click="setLocale('en')">EN</button>
            </div>
            <button class="refresh-button" :title="t('refresh')" :aria-label="t('refresh')" @click="refreshState">↻</button>
          </div>
        </div>

        <div class="view-scroll">
		  <div v-if="state.workspace && state.device_setup_required" class="device-setup-banner"><div><strong>请确认这台电脑的设备代码</strong><span>Launcher 无法仅凭当前证据安全判断本机是 A 还是 B。请选择一次；选择结果只保存在这台电脑，不随 Brain2 同步。</span></div><div class="button-row"><button class="secondary" :disabled="busy" @click="setLocalDevice('A')">这是设备 A</button><button class="secondary" :disabled="busy" @click="setLocalDevice('B')">这是设备 B</button></div></div>
		  <div v-else-if="state.workspace && !state.selected_device_local" class="remote-device-banner"><strong>当前查看设备 {{ state.selected_device_id }}</strong><span>这里显示同步到 Brain2 的远程历史记录。Agent 接入、重装、验证和本机扫描只能在设备 {{ state.local_device_id }} 上执行。</span><button class="text-button" @click="switchToLocalDevice">切回本机 →</button></div>
          <section v-if="activeView === 'product'" class="view-page manager-product-page">
            <template v-if="isBrain2">
              <header class="home-heading">
                <div>
                  <h2>{{ t('homeTitle') }}</h2>
                  <p>{{ t('homeIntro') }}</p>
                </div>
              </header>
              <div v-if="state.acceptance_mode === 'simulate-no-obsidian'" class="acceptance-banner"><strong>OB 安装验收模式</strong><span>Launcher 正在模拟“本机没有 OB”。不会卸载、删除或修改电脑里现有的 Obsidian，也不会覆盖正式版状态。</span></div>
              <div class="home-workbench">
                <section class="home-focus" aria-labelledby="home-next-title">
                  <div class="home-focus-meta">
                    <span>{{ locale === 'zh' ? '当前下一步' : 'Next action' }}</span>
                    <strong>{{ homeStage.step }}</strong>
                  </div>
                  <h3 id="home-next-title">{{ homeStage.title }}</h3>
                  <p>{{ homeStage.description }}</p>

                  <div v-if="homeStage.id === 'memory'" class="home-choice-actions">
                    <div>
                      <button class="secondary" :disabled="busy" @click="chooseAndAttachBrain2">{{ locale === 'zh' ? '连接已有仓库' : 'Connect existing' }}</button>
                      <small>{{ locale === 'zh' ? '只添加连接层，不迁移原有内容。' : 'Adds the connection layer without moving content.' }}</small>
                    </div>
                    <div>
                      <button class="primary" :disabled="busy" @click="chooseAndInstall">{{ locale === 'zh' ? '创建新仓库' : 'Create new' }}</button>
                      <small>{{ locale === 'zh' ? '在空文件夹中建立 Brain2 基础结构。' : 'Creates the Brain2 structure in an empty folder.' }}</small>
                    </div>
                  </div>
                  <div v-else-if="homeStage.id === 'obsidian'" class="home-stage-actions home-obsidian-actions">
                    <button class="primary" :disabled="busy" @click="detectObsidianInstallation">{{ locale === 'zh' ? (obsidianDetectionAttempted ? '重新检测' : '检测 Obsidian') : (obsidianDetectionAttempted ? 'Detect again' : 'Detect Obsidian') }}</button>
                    <button class="secondary" :disabled="busy" @click="prepareObsidianInstall">{{ locale === 'zh' ? '交给 Agent 安装' : 'Ask agent to install' }}</button>
                    <button v-if="obsidianDetectionAttempted" class="text-button" :disabled="busy" @click="chooseObsidianExecutable">{{ locale === 'zh' ? '手动定位' : 'Locate manually' }}</button>
                  </div>
                  <p v-if="homeStage.id === 'obsidian' && obsidianDetectionAttempted" class="home-detection-result" role="status">{{ locale === 'zh' ? '未检测到 Obsidian。你不需要知道安装路径；可重新检测或交给 Agent 安装。只有确认已经安装但仍未发现时，才需要手动定位。' : 'Obsidian was not detected. You do not need to know the install path: detect again or ask an agent to install it. Use manual location only when you know it is already installed.' }}</p>
                  <div v-else-if="homeStage.id === 'agent'" class="home-stage-actions">
                    <button class="primary" :disabled="!state.workspace" @click="setView('runtime')">{{ locale === 'zh' ? '连接 Agent' : 'Connect agent' }}</button>
                  </div>
                  <div v-else-if="homeStage.id === 'project'" class="home-stage-actions">
                    <button class="primary" :disabled="!state.workspace" @click="setView('projects')">{{ locale === 'zh' ? '开始识别项目' : 'Discover projects' }}</button>
                  </div>
                  <div v-else class="home-stage-actions">
                    <button class="primary" @click="setView('projects')">{{ locale === 'zh' ? '查看已连接项目' : 'View linked projects' }}</button>
                    <button class="secondary" @click="setView('health')">{{ locale === 'zh' ? '检查当前状态' : 'Review current status' }}</button>
                  </div>

                  <div v-if="state.workspace" class="home-path-row">
                    <span>{{ locale === 'zh' ? '当前记忆库' : 'Current memory' }}</span>
                    <code>{{ state.brain2_root }}</code>
                    <button class="text-button" @click="simpleAction('已打开 Brain2 文件夹', backend.openFolder)">{{ locale === 'zh' ? '打开文件夹' : 'Open folder' }}</button>
                  </div>
                </section>

                <aside class="home-progress" :aria-label="locale === 'zh' ? 'Brain2 设置进度' : 'Brain2 setup progress'">
                  <div class="home-progress-head">
                    <h3>{{ locale === 'zh' ? '设置进度' : 'Setup progress' }}</h3>
                    <span>{{ homeCompletedSteps }} / {{ homeSteps.length }}</span>
                  </div>
                  <ol class="home-step-list">
                    <li v-for="step in homeSteps" :key="step.id" :class="{ complete: step.complete, current: step.current }">
                      <span class="home-step-mark" aria-hidden="true">{{ step.complete ? '✓' : step.number }}</span>
                      <div>
                        <strong>{{ step.label }}</strong>
                        <small>{{ step.detail }}</small>
                      </div>
                      <button v-if="step.id === 'agent'" class="text-button" :disabled="!state.workspace" @click="setView('runtime')">{{ locale === 'zh' ? '查看' : 'View' }}</button>
                      <button v-else-if="step.id === 'project'" class="text-button" :disabled="!state.runtime_ready" @click="setView('projects')">{{ locale === 'zh' ? '查看' : 'View' }}</button>
                    </li>
                  </ol>
                </aside>

                <div class="home-disclosures">
                  <details class="home-commands">
                    <summary>
                      <span>
                        <strong id="home-command-title">{{ locale === 'zh' ? '常用指令' : 'Common prompts' }}</strong>
                      </span>
                      <span class="home-disclosure-mark" aria-hidden="true"></span>
                    </summary>
                    <div class="home-command-list" aria-labelledby="home-command-title">
                      <code>{{ locale === 'zh' ? '读取大脑，告诉我当前上下文和下一步。' : 'Read Brain2 and tell me the current context and next step.' }}</code>
                      <code>{{ locale === 'zh' ? '读取大脑，我想开始一个新项目；先和我确认目标，不要直接创建文件。' : 'Read Brain2. I want to start a project; confirm the goal with me before creating files.' }}</code>
                    </div>
                  </details>

                  <details class="home-summary">
                    <summary>
                      <span>
                        <strong>{{ locale === 'zh' ? '系统概览' : 'System summary' }}</strong>
                      </span>
                      <span class="home-disclosure-mark" aria-hidden="true"></span>
                    </summary>
                    <dl>
                      <div><dt>{{ t('workspace') }}</dt><dd>{{ workspaceLabel }}</dd></div>
                      <div><dt>{{ t('agents') }}</dt><dd>{{ runtimeLabel }}</dd></div>
                      <div><dt>{{ t('projectLinks') }}</dt><dd>{{ state.project_count }}</dd></div>
                      <div><dt>{{ t('checks') }}</dt><dd>{{ healthLabel }}</dd></div>
                    </dl>
                    <div class="home-summary-actions"><button class="secondary" @click="setView('health')">{{ locale === 'zh' ? '查看检查' : 'View checks' }}</button></div>
                  </details>
                </div>
              </div>
            </template>
            <template v-else>
              <div class="product-screen"><div class="section-title"><h1>{{ product.name }}</h1><p>该产品仍在规划中</p></div><div class="product-card"><div class="product-info"><div class="product-code">{{ product.code }}</div><h2>{{ product.name }}</h2><p>{{ product.description }}</p><div class="product-meta"><div class="meta"><small>STATE</small><strong class="orange">{{ product.status }}</strong></div><div class="meta"><small>VERSION</small><strong>—</strong></div><div class="meta"><small>MODE</small><strong>LOCAL FIRST</strong></div></div><button class="secondary" disabled>尚在规划中</button></div><div class="product-visual"><div class="orbit muted"><div class="core"><div><strong>···</strong><small>PLANNED</small></div></div></div></div></div></div>
            </template>
          </section>

          <section v-else-if="activeView === 'projects'" class="view-page">
            <div class="page-head"><span class="eyebrow">{{ t('projectEyebrow') }}</span><h2>{{ t('projectTitle') }}</h2><p>{{ t('projectIntro') }}</p></div>
            <div class="project-connect-panel project-scan-panel">
              <div class="project-scan-copy"><small>{{ t('howItWorks') }}</small><h3>{{ locale === 'zh' ? 'Agent 识别 → 你确认 → Agent 建档' : 'Agent identifies → you confirm → agent archives' }}</h3><p>{{ locale === 'zh' ? 'Agent 负责识别和调查；你只确认是否纳入，以及归属哪个项目。' : 'The agent identifies and investigates. You only confirm whether to include it and where it belongs.' }}</p><ol><li>{{ locale === 'zh' ? '选择 Agent 并生成任务' : 'Choose an agent and generate the task' }}</li><li>{{ locale === 'zh' ? '确认候选项目及归属' : 'Confirm candidates and ownership' }}</li><li>{{ locale === 'zh' ? '验证建档回执' : 'Verify the archive receipt' }}</li></ol><div class="project-fallback-actions"><button class="secondary" :disabled="busy || !state.workspace || !state.selected_device_local" @click="chooseProjectScanRoot">{{ locale === 'zh' ? '选择兜底目录' : 'Choose fallback folder' }}</button><button class="secondary" :disabled="busy || !state.workspace || !state.selected_device_local" @click="attachExternalProject">{{ locale === 'zh' ? '手动连接目录' : 'Connect a folder manually' }}</button><button class="secondary" :disabled="busy || !state.workspace || !state.selected_device_local" @click="startNewProjectWithAgent">{{ locale === 'zh' ? '与 Agent 开始新项目' : 'Start a project with an agent' }}</button></div></div>
              <div class="project-scan-control"><small>{{ t('singleStep') }}</small><strong>{{ locale === 'zh' ? '选择执行项目发现与建档的 Agent' : 'Choose the agent for project discovery and archiving' }}</strong><select v-model="scanAgentID" :disabled="busy || !state.workspace || !state.selected_device_local || !scanAgents.length"><option v-if="!scanAgents.length" value="">{{ locale === 'zh' ? '请先连接 Agent' : 'Connect an agent first' }}</option><option v-for="agent in scanAgents" :key="agent.id" :value="agent.id">{{ agent.name }}</option></select><div v-if="scanRootPath" class="scan-fallback-selected"><small>{{ locale === 'zh' ? '兜底范围已选' : 'Fallback selected' }}</small><code>{{ scanRootPath }}</code><button class="text-button" @click="scanRootPath = ''">{{ locale === 'zh' ? '清除兜底范围' : 'Clear fallback' }}</button></div><button class="primary" :disabled="busy || !state.workspace || !state.selected_device_local || !scanAgents.length" @click="prepareProjectScan">{{ locale === 'zh' ? '生成发现任务' : 'Generate discovery task' }}</button><button v-if="!scanAgents.length" class="secondary" :disabled="!state.workspace" @click="setView('runtime')">{{ locale === 'zh' ? '先连接 Agent' : 'Connect an agent first' }}</button></div>
            </div>
            <div v-if="projectScanResult" class="scan-handoff-panel compact"><div class="scan-handoff-head"><div><small>{{ t('promptReady') }}</small><h3>{{ locale === 'zh' ? '项目发现任务已生成' : 'Project discovery task generated' }}</h3><p>{{ projectScanResult.scan_root ? `${locale === 'zh' ? '兜底扫描范围' : 'Fallback scope'}：${projectScanResult.scan_root}` : (locale === 'zh' ? '发现范围：Brain2、GitHub 与本机证据' : 'Scope: Brain2, GitHub, and local evidence') }}</p></div><button class="secondary" @click="copyProjectScanPrompt">{{ locale === 'zh' ? '查看 / 复制 Prompt' : 'View / copy Prompt' }}</button></div><div class="scan-waiting"><span class="live-dot"></span><strong>{{ locale === 'zh' ? '等待 Agent 通过 MCP 回填候选' : 'Waiting for agent candidates through MCP' }}</strong><p>{{ locale === 'zh' ? 'Agent 完成后，候选会出现在下方；Manager 不会自动登记，也不会把候选当成完整档案。' : 'Candidates appear below when the agent finishes. Manager never registers them automatically or treats them as complete records.' }}</p></div></div>
            <div class="workflow-strip">
              <span>{{ locale === 'zh' ? 'Agent 识别' : 'AGENT DISCOVERY' }}</span><i>→</i>
              <span>{{ locale === 'zh' ? '候选项目' : 'CANDIDATES' }}</span><i>→</i>
              <span>{{ locale === 'zh' ? '用户选择' : 'USER CHOICE' }}</span><i>→</i>
              <span>{{ locale === 'zh' ? 'Agent 建档' : 'AGENT ARCHIVE' }}</span><i>→</i>
              <span>{{ locale === 'zh' ? '结果回执' : 'RECEIPT' }}</span>
            </div>
            <div v-if="pendingProjectCandidates.length" class="panel candidate-panel">
              <div class="panel-head"><div><small>{{ t('waitingForYou') }}</small><h3>{{ locale === 'zh' ? '待确认候选' : 'Pending candidates' }} · {{ pendingProjectCandidates.length }}</h3></div><div class="candidate-batch-actions"><button class="secondary" :disabled="busy || !archivablePendingCandidates.length" @click="toggleAllArchivableCandidates">{{ allArchivableCandidatesSelected ? (locale === 'zh' ? '取消全选' : 'Clear selection') : (locale === 'zh' ? '全选可建档' : 'Select archivable') }}</button><button class="primary" :disabled="busy || !scanAgents.length || !batchSelectedCandidateIDs.length" @click="prepareBatchProjectArchives">{{ locale === 'zh' ? '批量加入建档' : 'Archive selected' }}（{{ batchSelectedCandidateIDs.length }}）</button></div></div>
              <div class="candidate-list">
                <article v-for="candidate in pendingProjectCandidates" :key="candidate.id" :class="{ 'candidate-card-selected': batchCandidateSelections[candidate.id] }">
                  <div class="candidate-main"><label class="candidate-select-control" :class="{ disabled: !isArchivableCandidate(candidate) }"><input type="checkbox" :checked="!!batchCandidateSelections[candidate.id]" :disabled="busy || !isArchivableCandidate(candidate)" @change="toggleBatchCandidate(candidate.id)"><span>{{ isArchivableCandidate(candidate) ? '选择' : '不可批量' }}</span></label><div><small>{{ candidate.detected_kind || 'PROJECT' }} · 可信度 {{ Math.round(candidate.confidence * 100) }}%</small><h4>{{ candidate.name }}</h4><code>{{ candidate.root_path }}</code><small v-if="candidate.shared_repository_project_id">共享仓库：{{ candidate.shared_repository_project_id }} · 独立产品</small></div><div class="candidate-actions"><button class="primary" :title="isArchivableCandidate(candidate) ? '建立项目档案' : '仅代码、应用或产品进入建档'" :disabled="busy || !scanAgents.length || !isArchivableCandidate(candidate)" @click="confirmProjectCandidate(candidate.id)">{{ isArchivableCandidate(candidate) ? '加入建档' : '非产品项目' }}</button><button class="secondary" :disabled="busy" @click="ignoreProjectCandidate(candidate.id)">忽略</button></div></div>
                  <p v-if="candidate.suggested_goal"><strong>Agent 推测用途：</strong>{{ candidate.suggested_goal }}</p>
                  <ul><li v-for="evidence in candidate.evidence" :key="evidence">{{ evidence }}</li></ul><div class="candidate-parent-row"><span>如果它不是独立项目：</span><select v-model="candidateParentSelections[candidate.id]" :disabled="busy"><option value="">选择归属的已有项目</option><option v-for="project in state.projects" :key="project.id" :value="project.id">{{ project.name }}</option></select><button class="text-button" :disabled="busy || !candidateParentSelections[candidate.id]" @click="assignProjectCandidate(candidate.id)">归入该项目</button></div>
                </article>
              </div>
            </div>
            <div v-if="selectedProjectCandidates.length" class="panel candidate-panel archive-waiting-panel"><div class="panel-head"><div><small>{{ t('archiveTasks') }}</small><h3>{{ locale === 'zh' ? '等待 Agent 建档' : 'Waiting for agent archive' }} · {{ selectedProjectCandidates.length }}</h3></div></div><div class="archive-waiting-list"><div v-for="candidate in selectedProjectCandidates" :key="candidate.id"><strong>{{ candidate.name }}</strong><code>{{ candidate.root_path }}</code><span>{{ locale === 'zh' ? '已授权加入建档，等待 Agent 回填' : 'Authorized for archiving; waiting for agent backfill' }}</span><button class="secondary" :disabled="busy || !scanAgents.length" @click="confirmProjectCandidate(candidate.id)">{{ locale === 'zh' ? '重新生成建档任务' : 'Regenerate archive task' }}</button></div></div></div>
            <div class="panel project-panel">
              <div class="panel-head">
                <div><small>{{ t('registeredAssets') }}</small><h3>{{ locale === 'zh' ? '已登记项目' : 'Registered projects' }} · {{ state.project_count }}</h3></div>
                <div class="button-row">
                  <span v-if="ignoredProjectCandidates.length">已忽略 {{ ignoredProjectCandidates.length }} 个候选</span>
                </div>
              </div>
              <div v-if="state.projects.length" class="project-list">
                <div v-for="item in state.projects" :key="item.id">
                  <span>{{ item.mode === 'external' ? '↗' : '◆' }}</span>
                  <div class="project-copy">
                    <strong>{{ item.name }}</strong>
                     <small>{{ item.machine_record ? '仅有机器登记' : item.manifest_path ? 'Agent 已提交并验证项目档案' : '旧项目待补档' }} · {{ item.path_status === 'verified' ? '路径已验证' : item.path_status || '路径待检查' }}<template v-if="item.shared_repository_project_id"> · 与 {{ item.shared_repository_project_id }} 共享仓库</template><template v-if="item.source_path"> · {{ item.source_path }}</template></small>
                  </div>
                  <div class="project-actions">
                    <span v-if="item.machine_record" class="registration-boundary">仅登记，不修改内容</span>
                    <button class="project-product-add" :disabled="busy" :title="locale === 'zh' ? '打开联动产品' : 'Open linked products'" :aria-label="`${locale === 'zh' ? '为' : 'Open linked products for'} ${item.name}${locale === 'zh' ? '选择联动产品' : ''}`" @click="openProductLinkMenu(item.id)"></button>
                    <button v-if="!item.machine_record && item.manifest_path && item.mode === 'external'" class="secondary" :disabled="busy" @click="confirmProjectManifest(item)">重新定位</button>
                    <button v-else-if="!item.machine_record" class="secondary" :disabled="busy || !scanAgents.length" @click="prepareProjectArchive(item.id)">交给 Agent 提议补档</button>
                  </div>
                </div>
              </div>
              <div v-else class="empty-state">还没有正式项目。先审阅上方候选并确认一个测试项目；Protector 只接收 Brain2 已确认项目清单。</div>
            </div>
          </section>

          <section v-else-if="activeView === 'runtime'" class="view-page">
            <div class="page-head agent-head"><div><span class="eyebrow">{{ t('runtimeEyebrow') }}</span><h2>{{ t('runtimeTitle') }}</h2><p>{{ t('runtimeIntro') }}</p></div></div>
            <div class="agent-page-actions"><button class="secondary" :disabled="busy || !state.selected_device_local" @click="refreshAgents">{{ locale === 'zh' ? '重新检测本机环境' : 'Detect local environment' }}</button></div>
            <div class="prompt-config">
              <div class="prompt-config-copy"><strong>{{ t('workEntry') }}</strong><span>{{ locale === 'zh' ? '选择本次任务的上下文；该选择会写入受控 Prompt。' : 'Choose the context for this task. The selection is written into the controlled Prompt.' }}</span></div>
              <div class="prompt-entry-controls"><select v-model="promptIntent" :aria-label="locale === 'zh' ? '选择工作入口' : 'Choose work entry'" :disabled="busy"><option value="brain">{{ locale === 'zh' ? '读取大脑总控与当前上下文' : 'Read Brain2 control and current context' }}</option><option value="existing_project" :disabled="!state.projects.length">{{ locale === 'zh' ? '进入已有项目' : 'Enter an existing project' }}</option><option value="new_project">{{ locale === 'zh' ? '与用户讨论并建立新项目' : 'Discuss and establish a new project' }}</option></select><select v-if="promptIntent === 'existing_project'" v-model="promptProjectID" :aria-label="locale === 'zh' ? '选择项目' : 'Choose project'" :disabled="busy"><option value="">{{ locale === 'zh' ? '请选择项目' : 'Choose a project' }}</option><option v-for="item in state.projects" :key="item.id" :value="item.id">{{ item.name }}</option></select><div v-else-if="promptIntent === 'new_project'" class="prompt-boundary"><strong>{{ locale === 'zh' ? '由 Agent 与用户确认' : 'Confirmed by the user and agent' }}</strong><span>{{ locale === 'zh' ? 'Manager 不预填项目目标，也不替用户创建或整理内容。' : 'Manager does not prefill the goal or create and organize content for the user.' }}</span></div></div>
            </div>
            <div class="connection-legend"><span><i class="legend-dot detected"></i>{{ t('detected') }}</span><span><i class="legend-dot registered"></i>{{ t('connectorInstalled') }}</span><span><i class="legend-dot connected"></i>{{ t('memoryVerified') }}</span><span><i class="legend-dot fallback"></i>{{ t('promptAvailable') }}</span></div>
            <div class="agent-grid">
              <article v-for="agent in detections" :key="agent.id" class="agent-card" :class="{ registered: connectionFor(agent.id)?.status === 'registered', connected: connectionFor(agent.id)?.status === 'connected', missing: !agent.detected }">
                <div class="agent-card-top">
                  <div class="agent-identity"><span class="agent-mark" :class="`agent-${agent.id}`"><img :src="agentIcon(agent.id)" :alt="`${agent.name} logo`" /></span><div><small>{{ agent.surface.toUpperCase() }} / {{ agent.id.toUpperCase() }}</small><h3>{{ connectionFor(agent.id)?.name || agent.name }}</h3></div></div>
                  <span class="agent-state">{{ connectionLabel(agent.id) }}</span>
                </div>
                <p>{{ agentMessage(agent) }}</p>
                <dl><div><dt>{{ locale === 'zh' ? '检测' : 'Detection' }}</dt><dd>{{ state.selected_device_local ? (agent.detected ? (locale === 'zh' ? '已检测' : 'Detected') : (locale === 'zh' ? '未发现' : 'Not found')) : (locale === 'zh' ? '远程只读' : 'Remote read-only') }}</dd></div><div><dt>{{ locale === 'zh' ? '版本' : 'Version' }}</dt><dd>{{ state.selected_device_local ? (agent.version || '—') : '—' }}</dd></div></dl>
                <div class="agent-actions">
                  <button v-if="agent.quick_connect" class="primary" :disabled="busy || !state.workspace || !state.selected_device_local || !agent.detected" @click="runAgentPrimary(agent)">{{ agentPrimaryLabel(agent) }}</button>
                  <button v-if="agent.prompt_connect" class="secondary" :disabled="busy || !state.workspace || !state.selected_device_local" @click="preparePrompt(agent)">{{ promptButtonLabel(agent.id) }}</button>
                </div>
                <div class="agent-notices">
                  <div v-if="agent.mcp_status === 'update_available'" class="agent-update-status"><span>Brain2 连接器 {{ agent.mcp_version === 'legacy' ? '旧版' : `v${agent.mcp_version}` }} → v{{ agent.mcp_available_version }}</span><button class="text-button" @click="setView('settings')">前往设置</button></div>
                  <div v-else-if="agent.mcp_status === 'update_ignored'" class="agent-update-status ignored"><span>已忽略连接器 v{{ agent.mcp_available_version }} 提醒</span><button class="text-button" @click="setView('settings')">前往设置</button></div>
                  <div v-else-if="agent.mcp_status === 'conflict'" class="agent-update-status"><span>{{ locale === 'zh' ? '这个 Agent 目前连接的是另一套 Launcher。只有你点击“改用当前 Launcher”后才会切换。' : 'This agent is connected to another Launcher. It switches only after you choose “Use this Launcher”.' }}</span></div>
                  <div v-if="connectionFor(agent.id)?.last_error && connectionFor(agent.id)?.last_error !== agent.mcp_message" class="agent-error">{{ connectionFor(agent.id)?.last_error }}</div>
                </div>
              </article>
            </div>
            <details class="connection-note"><summary>{{ locale === 'zh' ? '安装与验证说明' : 'Installation and verification' }}</summary><p>{{ locale === 'zh' ? '安装连接器后仍需重启旧会话并执行开始 Prompt。只有 workspace_id、实际根目录和握手回执全部匹配，才会显示“当前记忆库已验证”。' : 'After installing the connector, restart older sessions and run the start Prompt. The memory is verified only after workspace_id, root path, and handshake receipt all match.' }}</p></details>
          </section>

          <section v-else-if="activeView === 'health'" class="view-page">
            <div class="page-head"><span class="eyebrow">{{ t('healthEyebrow') }}</span><h2>{{ t('healthTitle') }}</h2><p>{{ t('healthIntro') }}</p></div>
            <div class="health-summary" :class="health?.overall_status"><div><small>{{ locale === 'zh' ? '可用状态' : 'Usability' }}</small><strong>{{ healthLabel }}</strong></div><div><small>{{ locale === 'zh' ? '工作区 ID' : 'Workspace ID' }}</small><strong>{{ health?.workspace_id || '尚未生成' }}</strong></div></div>
            <div class="health-actions"><div><small>{{ locale === 'zh' ? '检查操作' : 'Check actions' }}</small><strong>先检查事实，再决定是否交给 Agent 判断</strong></div><div class="button-row"><button class="primary" :disabled="busy || !state.workspace" @click="runHealth">运行确定性检查</button><button class="secondary" :disabled="busy || !state.workspace" @click="prepareHealthPrompt">交给 Agent 分析</button></div></div>
            <div v-if="health" class="health-layers"><article v-for="layer in health.categories" :key="layer.category" :class="layer.status"><small>{{ healthCategoryLabel(layer.category) }}</small><strong>{{ layer.error + layer.warning ? `${layer.error + layer.warning} 项需关注` : '通过' }}</strong><span>{{ layer.ok }} 通过 · {{ layer.info }} 候选 · {{ layer.warning }} 警告 · {{ layer.error }} 错误</span></article></div>
            <div v-if="health" class="check-list"><div v-for="check in health.checks" :key="`${check.name}:${check.path}`" :class="check.status"><span>{{ check.status === 'ok' ? '✓' : check.status === 'info' ? '?' : '!' }}</span><div><strong>{{ check.name }}</strong><small>{{ check.message }} · {{ check.path }}</small></div></div></div>
          </section>

          <section v-else class="view-page">
            <div class="page-head"><span class="eyebrow">{{ t('settingsEyebrow') }}</span><h2>{{ t('settingsTitle') }}</h2><p>{{ t('settingsIntro') }}</p></div>
            <section class="settings-update" :class="{ available: applicationUpdate.status === 'update_available' || connectorUpdates.length, current: applicationUpdate.status === 'current' && updateCheckStatus === 'checked' && !connectorUpdates.length }" aria-live="polite">
              <div class="settings-update-heading">
                <div><h3>{{ locale === 'zh' ? '更新' : 'Updates' }}</h3><p>{{ settingsUpdateSummaryLabel }}</p></div>
                <div class="settings-update-actions">
                  <button v-if="applicationUpdate.status === 'update_available'" class="primary" :disabled="busy || openingLauncherUpdate" @click="runApplicationUpdateAction">{{ applicationUpdateActionLabel }}</button>
                  <button v-if="connectorUpdates.length" class="primary" :disabled="busy" @click="updateAllConnectors">{{ locale === 'zh' ? '更新连接组件' : 'Update connectors' }}</button>
                  <button class="secondary" :disabled="busy" @click="runSettingsUpdateAction">{{ updateActionLabel }}</button>
                </div>
              </div>
              <div class="settings-update-status">
                <div><span>Brain2 Manager</span><strong>{{ applicationUpdateLabel }}</strong><small>{{ applicationUpdateVersionLabel }}</small></div>
                <div><span>{{ locale === 'zh' ? 'Brain2 连接组件' : 'Brain2 connectors' }}</span><strong>{{ locale === 'zh' ? updateSummaryLabel : updateCheckStatus === 'checked' && !connectorUpdates.length ? 'Up to date' : updateCheckStatus === 'checking' ? 'Checking' : connectorUpdates.length ? 'Update available' : 'Not checked' }}</strong><small>{{ locale === 'zh' ? updateSummaryHint : updateCheckedAt ? `Last checked: ${updateCheckedAt}` : 'Not checked on this device' }}</small></div>
              </div>
              <details v-if="applicationUpdate.status === 'update_available' && applicationUpdate.release?.release_notes" class="settings-update-notes"><summary>{{ locale === 'zh' ? '查看版本说明' : 'View release notes' }}</summary><p>{{ applicationUpdate.release.release_notes }}</p></details>
            </section>
            <div class="settings-actions"><article class="device-management-card"><small>{{ t('deviceManagement') }}</small><h3>设备管理</h3><p>{{ localDeviceSourceLabel }}。设备代码是稳定身份，括号内是实际 Windows 设备名。</p><div class="device-management-list"><div v-for="device in state.devices" :key="device.id"><span><strong>{{ device.id }}{{ device.machine_name ? `（${device.machine_name}）` : '' }}</strong><small>{{ device.local ? '本机设备' : '远程设备' }} · {{ device.brain2_path }}</small></span><button v-if="!device.local" class="text-button danger" :disabled="busy" @click="removeManagedDevice(device.id)">移出列表</button><b v-else>本机</b></div></div><button v-if="missingDeviceID" class="secondary" :disabled="busy" @click="addMissingDevice">添加设备 {{ missingDeviceID }}</button><span v-else class="all-devices-added">A/B 设备均已登记</span></article><article><small>{{ t('currentMemory') }}</small><h3>{{ state.brain2_root || '尚未设置' }}</h3><p>默认只使用一个协同记忆库。Manager 不删除、迁移或整理其中的用户内容。</p><button class="secondary" :disabled="!state.workspace" @click="simpleAction('已打开协同记忆库', backend.openFolder)">打开当前记忆库</button></article><article><small>{{ t('multiMemory') }}</small><h3>连接另一个记忆库</h3><p>大多数用户不需要。只有确实要维护多套独立记忆时才使用；Agent 连接器无需重新安装。</p><button class="secondary" @click="chooseAndAttachBrain2">选择另一个 Obsidian 仓库</button></article><article><small>{{ t('obsidian') }}</small><h3>{{ state.obsidian_ready ? '已检测到' : '尚未检测到' }}</h3><p>{{ state.obsidian_path || '选择期望位置，生成给 Agent 的官方安装任务。' }}</p><div class="button-row"><button v-if="state.obsidian_ready" class="secondary" :disabled="!state.workspace" @click="simpleAction('已请求 Obsidian 打开 Brain2', backend.openObsidian)">打开 Obsidian</button><button v-else class="primary" @click="prepareObsidianInstall">交给 Agent 安装</button></div></article><article><small>{{ t('agentAccess') }}</small><h3>旧内容默认只读</h3><p>Manager 不移动、不重命名、不删除用户资产。语义判断由 Agent 完成，高风险动作必须再次询权。</p><button class="secondary" :disabled="!state.workspace" @click="setView('runtime')">管理 Agent 连接</button></article><article><small>{{ t('diagnostics') }}</small><h3>确定性检查</h3><p>检查结果保存在 Brain2 的机器目录，并可作为证据交给 Agent 分析。</p><button class="secondary" :disabled="!state.workspace" @click="setView('health')">进入检查与维护</button></article></div>
          </section>
        </div>

      </main>

      <Transition name="progress-fade">
        <div v-if="connectionProgress" class="connection-progress-overlay" role="status" aria-live="polite">
          <div class="connection-progress-card">
            <div class="logo-loader" :class="{ failed: connectionProgress.phase === 'failed', complete: connectionProgress.phase === 'connected' }">
              <span class="loader-orbit orbit-a"></span>
              <span class="loader-orbit orbit-b"></span>
              <img src="/assets/meow-forge-symbol-reverse-hd.png" alt="Meow Launcher 正在连接" />
            </div>
            <span class="progress-kicker">{{ connectionProgress.agent_name }} / {{ elapsedLabel }}</span>
            <h3>{{ connectionProgress.title }}</h3>
            <p>{{ connectionProgress.detail }}</p>
            <div class="progress-steps">
              <div v-for="(step, index) in progressSteps" :key="step" :class="{ done: progressStepIndex > index, active: progressStepIndex === index }">
                <i>{{ progressStepIndex > index ? '✓' : index + 1 }}</i><span>{{ step }}</span>
              </div>
            </div>
            <small>纯本地检查，不会启动模型。完成后复制开始 Prompt，交给你实际使用的 Agent 会话。</small>
          </div>
        </div>
      </Transition>
      <Transition name="progress-fade">
        <div v-if="productLinkProject" class="prompt-modal-overlay" role="dialog" aria-modal="true" aria-labelledby="product-link-title" @click.self="closeProductLinkMenu">
          <div class="prompt-modal-card product-link-modal">
            <div class="prompt-modal-head">
              <div>
                <small>{{ locale === 'zh' ? '产品联动' : 'LINKED PRODUCTS' }}</small>
                <h3 id="product-link-title">{{ productLinkProject.name }}</h3>
                <p>{{ locale === 'zh' ? '选择要与这个项目一起使用的产品。每个产品仍按自己的权限边界运行。' : 'Choose a product to use with this project. Each product keeps its own permission boundary.' }}</p>
              </div>
              <button class="prompt-close" :title="locale === 'zh' ? '关闭' : 'Close'" @click="closeProductLinkMenu">×</button>
            </div>
            <div class="product-link-options">
              <button v-for="option in productLinkOptions" :key="option.id" class="product-link-option" :disabled="busy || !option.available" @click="openLinkedProduct(option.id)">
                <span>
                  <strong>{{ option.name }}</strong>
                  <small>{{ option.description }}</small>
                </span>
                <b>{{ option.status }}</b>
              </button>
            </div>
            <div class="prompt-modal-actions">
              <span>{{ locale === 'zh' ? '这里是统一入口，不会绕过产品授权或改变项目内容。' : 'This is a unified entry point; it never bypasses product authorization or changes project content.' }}</span>
              <div class="button-row"><button class="secondary" @click="closeProductLinkMenu">{{ locale === 'zh' ? '关闭' : 'Close' }}</button></div>
            </div>
          </div>
        </div>
      </Transition>
      <Transition name="progress-fade">
        <div v-if="brainAttachConfirmation" class="prompt-modal-overlay" role="dialog" aria-modal="true" aria-labelledby="brain-attach-title">
          <div class="prompt-modal-card brain-confirm-card">
            <div class="prompt-modal-head"><div><small>INCREMENTAL CONNECTION</small><h3 id="brain-attach-title">确认接入这个 Obsidian 仓库</h3><p>{{ brainAttachConfirmation.root }}</p></div><button class="prompt-close" title="关闭" :disabled="busy" @click="brainAttachConfirmation = null">×</button></div>
            <div class="brain-confirm-body">
              <strong>Launcher 只新增 Brain2 连接层</strong>
              <ul><li>在 <code>.brain2</code> 中创建本机配置、空机器索引和运行目录；现有 Brain2 入口继续沿用。</li><li>不会移动、重命名、分类、删除或改写原有笔记。</li><li>不会自动扫描全部内容、建立项目清单或让 Agent 替你治理旧资料。</li></ul>
              <p>接入完成后，这台电脑上的 Agent 将通过 Launcher 注册表识别这套协同记忆库。</p>
            </div>
            <div class="prompt-modal-actions"><span>这是本机连接配置变更；你可以取消而不产生任何写入。</span><div class="button-row"><button class="secondary" :disabled="busy" @click="brainAttachConfirmation = null">取消</button><button class="primary" :disabled="busy" @click="confirmAttachBrain2">确认增量接入</button></div></div>
          </div>
        </div>
      </Transition>
      <Transition name="progress-fade">
        <div v-if="promptModal" class="prompt-modal-overlay" role="dialog" aria-modal="true" aria-labelledby="active-prompt-title">
          <div class="prompt-modal-card" :class="{ 'has-update-summary': promptModal.updateSummary }">
            <div class="prompt-modal-head"><div><small>{{ promptModal.eyebrow }}</small><h3 id="active-prompt-title">{{ promptModal.title }}</h3><p>{{ promptModal.subtitle }}</p></div><button class="prompt-close" title="关闭" @click="closePromptModal">×</button></div>
            <div v-if="promptModal.updateSummary" class="update-install-summary" :class="{ conflict: promptModal.updateSummary.conflict }"><div><small>SKILL</small><strong>{{ promptModal.updateSummary.skill }}</strong></div><div><small>MCP HOST</small><strong>{{ promptModal.updateSummary.host }}</strong></div><div><small>CONFIG</small><strong>{{ promptModal.updateSummary.config }}</strong></div><div><small>AGENT</small><strong>{{ promptModal.updateSummary.verification }}</strong></div></div>
            <textarea v-if="promptModal.prompt" :value="promptModal.prompt" readonly></textarea>
            <div class="prompt-modal-actions"><span v-if="promptModal.prompt" class="prompt-copy-status" :class="{ success: promptModal.copied }">{{ promptModal.copied ? '✓ 检查 Prompt 已复制。重启 Agent 后直接粘贴。' : '尚未复制；请点击右侧按钮。' }}</span><span v-else>现有文件保持不变。Launcher 没有静默覆盖用户内容。</span><div class="button-row"><button class="secondary" @click="closePromptModal">稍后处理</button><button v-if="promptModal.prompt" class="primary prompt-copy-button" :class="{ copied: promptModal.copied }" @click="copyActivePrompt">{{ promptModal.copied ? '✓ 已复制' : '复制检查 Prompt' }}</button></div></div>
          </div>
        </div>
      </Transition>
    </section>
  </div>
</template>
