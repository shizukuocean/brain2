<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { backend, type LauncherProductState, type LauncherSurfaceState } from './backend'
import { EventsOn } from '../../wailsjs/runtime/runtime'

type ViewName = 'product' | 'catalog' | 'settings'
type Locale = 'zh' | 'en'
type DownloadProgress = { product_id: string; phase: string; bytes: number; total_bytes: number }
type LauncherUpdateRequest = { product_id?: string; return_action?: string }

const savedLocale = window.localStorage.getItem('meow-launcher-locale')
const locale = ref<Locale>(savedLocale === 'en' || savedLocale === 'zh'
  ? savedLocale
  : window.navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en')

const copy = {
  zh: {
    navProduct: '应用详细',
    navUninstall: '应用列表',
    navSettings: '设置',
    hubTitle: '产品中心',
    hubSub: '全部 Meow 产品',
    refresh: '刷新状态',
    ready: '就绪',
    working: '处理中',
    footer: '集成产品安装器',
    products: '款产品',
    productEyebrow: '应用详细',
    productTitle: '应用详细',
    productIntro: '在同一页切换全部产品，查看本机版本、目录版本和更新状态。产品内部组件仍由各自应用维护。',
    selectProduct: '切换应用',
    installedVersion: '本机版本',
    catalogVersion: '目录版本',
    latestVersion: '最新版本',
    updateStatus: '更新状态',
    authorizationStatus: '安装授权',
    releaseNotes: '版本说明',
    localResult: '来自本机检测',
    notInstalled: '本机未安装',
    channel: '发布通道',
    waitingCheck: '待检查',
    neverChecked: '尚未进行在线检测',
    openProduct: '打开产品',
    launching: '启动中…',
    checkToInstall: '检查后安装',
    downloadPending: '下载服务待接入',
    planned: '产品规划中',
    checkUpdate: '检查产品更新',
    checking: '检查中…',
    prepareUpdate: '下载并校验安装包',
    preparingUpdate: '正在下载并校验…',
    installState: '安装准备',
    readyInstall: '安装包已验证',
    installNotStarted: '等待操作',
    installUnavailable: '当前不可安装',
    installAuthorizationRequired: '等待授权',
    installDownloading: '正在下载',
    installDownloadFailed: '下载失败，可重试',
	installClosingProduct: '正在关闭当前产品',
	installProductStillRunning: '等待你关闭产品',
    installNotReady: '尚未准备完成',
    installFailed: '安装失败',
    installStarting: '正在启动安装程序',
    installLocationRequired: '请选择安装磁盘',
    installLocationSelected: '安装磁盘已选择',
    installInstalled: '已安装',
    downloadSlowHint: '正在连接并下载。若 45 秒没有收到任何数据，Launcher 会安全停止并允许重试。',
	installNow: '关闭并更新',
    installProductNow: '立即安装',
	startingInstaller: '正在关闭并更新…',
    chooseInstallLocation: '选择安装磁盘',
    choosingInstallLocation: '正在选择磁盘…',
    chooseInstallLocationThenDownload: '请先在设置中选择 Meow System 安装位置。',
    downloadConnecting: '正在连接下载服务器…',
    downloadVerifying: '下载完成，正在校验文件…',
    installLocation: '安装目录',
	closeBeforeInstall: '点击后 Launcher 会请求当前产品正常退出，确认退出后才替换文件，并且只重新打开一个新版窗口。若产品无法正常退出，安装会停止并请你手动关闭；不会强制结束进程。',
	availableNow: '当前可安装',
    boundary: '更新分工',
    launcherBoundaryTitle: 'Launcher 负责产品本体',
    launcherBoundaryText: '检查、下载、安装和更新 Brain2 Manager、Meowdesk、Protector、Helper 等桌面产品。',
    managerBoundaryTitle: 'Brain2 Manager 负责连接能力',
    managerBoundaryText: '继续维护 Brain2 Skill、MCP Host、Agent 配置、备份与回滚，跨产品联动保持不变。',
    uninstallEyebrow: '应用列表',
    uninstallTitle: '应用列表',
    uninstallIntro: '应用卸载与用户数据清理严格分开。默认只移除产品文件，不删除内容、记忆库或备份。',
    installedProduct: '选择已安装产品',
    selectedProduct: '当前产品',
    version: '版本',
    unknown: '未知',
    uninstallProduct: '卸载产品',
    uninstalling: '卸载中…',
    willRemove: '本次会删除',
    willKeep: '这些会保留',
    cleanupTitle: '备份与数据清理',
    cleanupText: '深度清理需要安装清单、备份确认和可恢复策略。当前不会提供一键删除用户数据。',
    cleanupPending: '安全清理待后续接入',
    noUninstall: '当前没有可由 Launcher 卸载的已安装产品',
    catalogVersionLabel: '应用版本',
    catalogDownload: '下载',
    catalogDetails: '应用详细',
    catalogUninstall: '卸载',
    settingsEyebrow: '安装与账号',
    settingsTitle: '设置',
    systemLocationTitle: '应用安装位置',
    systemLocationText: '选择一个位置，Launcher 会在其中创建 Meow System。以后安装应用不再重复询问。',
    systemLocationUnset: '尚未选择',
    chooseSystemLocation: '选择位置',
    changeSystemLocation: '更改位置',
    choosingSystemLocation: '正在选择…',
    accountSectionTitle: '账号',
    accountTitle: '公开 Preview 无需登录',
    accountIntro: '当前公开 Preview 可以直接检查、下载和安装。账号功能尚未接入，未来只用于需要会员或购买资格的版本。',
    accountState: '账号状态',
    accountSignedOut: '账号功能尚未接入',
    accountSignedIn: '已登录',
    accountText: '公开 Preview 无需登录；需要授权的版本以后在这里接入账号。',
    loginPending: '登录服务待接入',
    installed: '已安装',
    availableAfterCheck: '检查后可安装',
    statusNotChecked: '尚未检查',
    statusNotInstalled: '尚未安装',
    statusPlanned: '规划中',
    statusUnavailable: '更新源未接入',
    statusCurrent: '已是当前版本',
    statusUpdate: '发现新版本',
    statusDownload: '可供下载',
    statusLauncherUpdate: '请先更新 Launcher',
    messageNotChecked: '尚未检查产品更新',
    messageNotInstalled: '安装后可检查更新',
    messagePlanned: '产品仍在规划中',
    messageUnavailable: '在线更新源尚未接入',
    messageCurrent: '本机已经是最新版本',
    messageUpdate: '检测到可用的新版本',
    messageDownload: '产品可供下载安装',
    messageLauncherUpdate: '当前版本过旧，请先更新 Launcher',
    authAuthorized: '本次已授权',
    authPending: '检查版本后确认',
    confirmPrefix: '确认卸载',
    confirmBody: '本次只删除已登记的产品应用文件，不删除用户内容、Brain2 记忆库或 Launcher 备份。请先关闭该产品。',
    launchDone: '已启动',
    updateChecking: '正在检查产品更新…',
    uninstallDone: '已卸载，用户数据保持不变',
    genericError: '操作没有完成，请检查安装状态后重试。',
    preserveContent: '用户创建的内容与项目数据',
    preserveBrain: 'Brain2 记忆库与 .brain2 配置',
    preserveBackup: 'Launcher 备份与账号记录',
  },
  en: {
    navProduct: 'App details',
    navUninstall: 'App list',
    navSettings: 'Settings',
    hubTitle: 'Product hub',
    hubSub: 'All Meow products',
    refresh: 'Refresh',
    ready: 'Ready',
    working: 'Working',
    footer: 'Unified product installer',
    products: 'products',
    productEyebrow: 'App details',
    productTitle: 'App details',
    productIntro: 'Switch between products, compare local and catalog versions, and check application updates. Each product still maintains its own internal components.',
    selectProduct: 'Switch app',
    installedVersion: 'Installed version',
    catalogVersion: 'Catalog version',
    latestVersion: 'Latest version',
    updateStatus: 'Update status',
    authorizationStatus: 'Install access',
    releaseNotes: 'Release notes',
    localResult: 'Detected on this device',
    notInstalled: 'Not installed',
    channel: 'Release channel',
    waitingCheck: 'Pending check',
    neverChecked: 'No online check yet',
    openProduct: 'Open product',
    launching: 'Opening…',
    checkToInstall: 'Check to install',
    downloadPending: 'Download service pending',
    planned: 'Product planned',
    checkUpdate: 'Check for updates',
    checking: 'Checking…',
    prepareUpdate: 'Download & verify installer',
    preparingUpdate: 'Downloading & verifying…',
    installState: 'Install preparation',
    readyInstall: 'Installer verified',
    installNotStarted: 'Waiting for action',
    installUnavailable: 'Not available to install',
    installAuthorizationRequired: 'Authorization required',
    installDownloading: 'Downloading',
    installDownloadFailed: 'Download failed; retry available',
	installClosingProduct: 'Closing the current product',
	installProductStillRunning: 'Waiting for you to close the product',
    installNotReady: 'Not ready',
    installFailed: 'Installation failed',
    installStarting: 'Starting installer',
    installLocationRequired: 'Choose an install drive',
    installLocationSelected: 'Install drive selected',
    installInstalled: 'Installed',
    downloadSlowHint: 'Connecting and downloading. If no data arrives for 45 seconds, Launcher stops safely and lets you retry.',
	installNow: 'Close and update',
    installProductNow: 'Install now',
	startingInstaller: 'Closing and updating…',
    chooseInstallLocation: 'Choose install drive',
    choosingInstallLocation: 'Choosing drive…',
    chooseInstallLocationThenDownload: 'Choose the Meow System location in Settings first.',
    downloadConnecting: 'Connecting to the download server…',
    downloadVerifying: 'Download complete; verifying the file…',
    installLocation: 'Install folder',
	closeBeforeInstall: 'Launcher will ask the current product to close normally, wait for it to exit, replace its files, and reopen exactly one updated window. If it cannot exit normally, installation stops and asks you to close it manually; the process is never force-killed.',
	availableNow: 'Available now',
    boundary: 'Update ownership',
    launcherBoundaryTitle: 'Launcher updates applications',
    launcherBoundaryText: 'Checks, downloads, installs, and updates Brain2 Manager, Meowdesk, Protector, Helper, and other desktop products.',
    managerBoundaryTitle: 'Brain2 Manager updates connectors',
    managerBoundaryText: 'Continues to maintain Brain2 Skills, MCP Hosts, agent configuration, backups, and rollback without breaking cross-product links.',
    uninstallEyebrow: 'App list',
    uninstallTitle: 'App list',
    uninstallIntro: 'Application removal and user-data cleanup are kept separate. By default, only product files are removed; content, memory, and backups stay intact.',
    installedProduct: 'Installed product',
    selectedProduct: 'Selected product',
    version: 'Version',
    unknown: 'Unknown',
    uninstallProduct: 'Uninstall product',
    uninstalling: 'Uninstalling…',
    willRemove: 'Will be removed',
    willKeep: 'Will be kept',
    cleanupTitle: 'Backup & data cleanup',
    cleanupText: 'Deep cleanup requires an install manifest, backup confirmation, and a recovery policy. One-click deletion of user data is not available.',
    cleanupPending: 'Safe cleanup coming later',
    noUninstall: 'No installed product can currently be removed by Launcher',
    catalogVersionLabel: 'App version',
    catalogDownload: 'Download',
    catalogDetails: 'App details',
    catalogUninstall: 'Uninstall',
    settingsEyebrow: 'Install & account',
    settingsTitle: 'Settings',
    systemLocationTitle: 'App installation location',
    systemLocationText: 'Choose a location and Launcher creates Meow System inside it. Future app installs will not ask again.',
    systemLocationUnset: 'Not selected',
    chooseSystemLocation: 'Choose location',
    changeSystemLocation: 'Change location',
    choosingSystemLocation: 'Choosing…',
    accountSectionTitle: 'Account',
    accountTitle: 'Public Preview needs no sign-in',
    accountIntro: 'Public Preview builds can be checked, downloaded, and installed directly. Sign-in is not implemented yet and will only be required for protected releases.',
    accountState: 'Account status',
    accountSignedOut: 'Sign-in not implemented',
    accountSignedIn: 'Signed in',
    accountText: 'Public Preview needs no sign-in. Account access will appear here when protected releases require it.',
    loginPending: 'Sign-in service pending',
    installed: 'Installed',
    availableAfterCheck: 'Available after check',
    statusNotChecked: 'Not checked',
    statusNotInstalled: 'Not installed',
    statusPlanned: 'Planned',
    statusUnavailable: 'Source unavailable',
    statusCurrent: 'Up to date',
    statusUpdate: 'Update available',
    statusDownload: 'Available to download',
    statusLauncherUpdate: 'Update Launcher first',
    messageNotChecked: 'Product updates have not been checked',
    messageNotInstalled: 'Install the product to check for updates',
    messagePlanned: 'This product is still on the roadmap',
    messageUnavailable: 'The online update source is not connected yet',
    messageCurrent: 'The installed version is current',
    messageUpdate: 'A newer version is available',
    messageDownload: 'This product is available to download',
    messageLauncherUpdate: 'Update Launcher before installing this product',
    authAuthorized: 'Authorized',
    authPending: 'Confirmed after version check',
    confirmPrefix: 'Uninstall',
    confirmBody: 'Only registered application files will be removed. User content, the Brain2 vault, and Launcher backups will remain. Close the product first.',
    launchDone: 'opened',
    updateChecking: 'Checking for product updates…',
    uninstallDone: 'was removed; user data remains unchanged',
    genericError: 'The operation did not finish. Check the installation and try again.',
    preserveContent: 'User-created content and project data',
    preserveBrain: 'Brain2 vault and .brain2 configuration',
    preserveBackup: 'Launcher backups and account records',
  },
} as const

type CopyKey = keyof typeof copy.zh
const t = (key: CopyKey) => copy[locale.value][key]

const downloadProgress = ref<DownloadProgress | null>(null)
const downloadStartedAt = ref(0)
const updateActionArea = ref<HTMLElement | null>(null)
const launcherScroll = ref<HTMLElement | null>(null)
let stopDownloadProgress: (() => void) | undefined
let stopUpdateRequest: (() => void) | undefined

function receiveDownloadProgress(progress: DownloadProgress) {
  downloadProgress.value = progress
  if (!downloadStartedAt.value && progress.phase === 'downloading') downloadStartedAt.value = Date.now()
}

async function revealUpdateArea() {
  await nextTick()
  updateActionArea.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
}

async function receiveLauncherUpdateRequest(request: LauncherUpdateRequest) {
  requestedUpdateHandled.value = false
  activeView.value = 'product'
  if (request?.product_id) selectedProductId.value = request.product_id
  statusText.value = locale.value === 'zh'
    ? '已从应用收到更新请求，正在打开对应产品更新页…'
    : 'Update request received. Opening the matching product page…'
  await refreshState()
  await revealUpdateArea()
}

const downloadPercent = computed(() => {
  const progress = downloadProgress.value
  if (!progress || progress.total_bytes <= 0) return 0
  return Math.min(100, Math.round(progress.bytes / progress.total_bytes * 100))
})

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 MB'
  return `${(bytes / 1024 / 1024).toFixed(bytes >= 100 * 1024 * 1024 ? 1 : 2)} MB`
}

function downloadSpeed() {
  const progress = downloadProgress.value
  if (!progress || !downloadStartedAt.value || progress.bytes <= 0) return ''
  const seconds = Math.max(1, (Date.now() - downloadStartedAt.value) / 1000)
  return `${formatBytes(progress.bytes / seconds)}/s`
}

const descriptions: Record<string, Record<Locale, string>> = {
  'brain2-manager': {
    zh: '本地外接大脑与多 Agent 协作系统。工作区、项目、Agent 与跨产品联动都在独立的 Manager 中运行。',
    en: 'A local external-memory and multi-agent collaboration system. Workspaces, projects, agents, and cross-product links run in the independent Manager.',
  },
  'meow-launcher': {
    zh: 'Meow 系列产品统一的安装、授权、更新、启动与恢复入口。',
    en: 'The unified install, access, update, launch, and recovery entry point for Meow products.',
  },
  meowdesk: {
    zh: '可由用户与 Agent 驱动、组合和扩展的个人工作台。',
    en: 'A personal workspace that users and agents can drive, compose, and extend.',
  },
  'meow-viewer': {
    zh: 'Meow 系列内容与成果的统一查看器。',
    en: 'A unified viewer for content and output across the Meow product family.',
  },
  'meow-protector': {
    zh: '面向本地项目的只读检查、风险验证与保护能力。',
    en: 'Read-only inspection, risk verification, and protection for local projects.',
  },
  'dev-helper': {
    zh: '面向开发规格、问题证据与人工验收闭环的本地桌面辅助产品。',
    en: 'A local desktop assistant for specifications, issue evidence, and human acceptance workflows.',
  },
}

const productIconSources: Record<string, string> = {
  'brain2-manager': '/assets/brain2-manager-icon.png',
  'meow-launcher': '/assets/meow-launcher-icon.png',
  'dev-helper': '/assets/dev-helper-icon.png',
}

const state = ref<LauncherSurfaceState>({
  products: [],
  authenticated: false,
  account_status: '',
  meow_system_root: '',
  meow_system_status: 'required',
  meow_system_message: '',
  launcher_build: 'v0.6.6 WIP',
  download_service: 'planned',
  update_history_status: 'ready',
  update_history_message: '',
})
const activeView = ref<ViewName>('product')
const selectedProductId = ref('brain2-manager')
const busyAction = ref('')
const statusText = ref('')
const requestedUpdateHandled = ref(false)

const selectedProduct = computed(() => state.value.products.find(product => product.id === selectedProductId.value) ?? state.value.products[0])
const toolbarTitle = computed(() => activeView.value === 'product' ? t('navProduct') : activeView.value === 'catalog' ? t('navUninstall') : t('navSettings'))
const statusDisplay = computed(() => busyAction.value ? t('working') : statusText.value || t('ready'))
const accountStatus = computed(() => state.value.authenticated ? t('accountSignedIn') : t('accountSignedOut'))

function setLocale(next: Locale) {
  locale.value = next
  window.localStorage.setItem('meow-launcher-locale', next)
  document.documentElement.lang = next === 'zh' ? 'zh-CN' : 'en'
  statusText.value = ''
}

function localError(error?: unknown) {
  if (typeof error === 'string' && error.trim()) return error.trim()
  if (error instanceof Error && error.message.trim()) return error.message.trim()
  if (error && typeof error === 'object' && 'message' in error) {
    const message = String((error as { message?: unknown }).message ?? '').trim()
    if (message) return message
  }
  return t('genericError')
}

function productDescription(product: LauncherProductState) {
  return descriptions[product.id]?.[locale.value] ?? product.description
}

function productIconSource(product: LauncherProductState) {
  return productIconSources[product.id] ?? ''
}

function productIconFallback(product: LauncherProductState) {
  return Array.from(product.name || '?').slice(0, 2).join('').toUpperCase()
}

function channelLabel(channel: string) {
  if (locale.value === 'en') return channel
  const labels: Record<string, string> = { preview: '预览', development: '开发', roadmap: '规划' }
  return labels[channel] ?? channel
}

function localizedVersion(value?: string) {
  if (!value) return '—'
  if (locale.value === 'en') {
    if (value === '待发布') return 'Pending release'
    if (value === '未发布') return 'Unreleased'
  }
  return value
}

function formatDateTime(value?: string) {
  if (!value) return t('neverChecked')
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(parsed)
}

async function refreshState() {
  try {
    state.value = await backend.state()
    if (!state.value.products.some(product => product.id === selectedProductId.value)) {
      selectedProductId.value = state.value.products[0]?.id ?? ''
    }
	if (!requestedUpdateHandled.value && state.value.requested_product_id && state.value.products.some(product => product.id === state.value.requested_product_id)) {
	  selectedProductId.value = state.value.requested_product_id
	  activeView.value = 'product'
	  requestedUpdateHandled.value = true
	  const requested = state.value.products.find(product => product.id === selectedProductId.value)
	  if (requested) {
		const checked = await checkProductUpdate(requested)
		if (checked?.installed && ['update_available', 'download_available'].includes(checked.update_status) && checked.install_status !== 'ready_to_install') {
		  await prepareProductUpdate(checked)
		}
	  }
	}
  } catch (error) {
    statusText.value = localError(error)
  }
}

function setView(view: ViewName) {
  activeView.value = view
}

async function chooseMeowSystemRoot() {
  if (busyAction.value) return
  busyAction.value = 'system-location'
  try {
    state.value = await backend.chooseMeowSystemRoot()
    statusText.value = state.value.meow_system_root
      ? `${t('systemLocationTitle')} · ${state.value.meow_system_root}`
      : state.value.meow_system_message
  } catch (error) {
    statusText.value = localError(error)
  } finally {
    busyAction.value = ''
  }
}

async function openProductDetail(product: LauncherProductState, focusAction = false) {
  selectedProductId.value = product.id
  activeView.value = 'product'
  await nextTick()
  if (focusAction) {
    await revealUpdateArea()
    return
  }
  launcherScroll.value?.scrollTo({ top: 0, behavior: 'smooth' })
}

async function launchProduct(product: LauncherProductState) {
  if (busyAction.value || !product.can_launch) return
  busyAction.value = `launch:${product.id}`
  try {
    await backend.launchProduct(product.id)
    statusText.value = `${product.name} ${t('launchDone')}`
  } catch (error) {
    statusText.value = localError(error)
  } finally {
    busyAction.value = ''
  }
}

async function checkProductUpdate(product: LauncherProductState) {
  if (busyAction.value || product.release_status === 'planned') return null
  busyAction.value = `update:${product.id}`
  statusText.value = `${product.name} · ${t('updateChecking')}`
  try {
    const checked = await backend.checkProductUpdate(product.id)
    const index = state.value.products.findIndex(item => item.id === checked.id)
    if (index >= 0) state.value.products[index] = checked
    statusText.value = updateMessage(checked)
	return checked
  } catch (error) {
    statusText.value = localError(error)
	return null
  } finally {
    busyAction.value = ''
  }
}

async function prepareProductUpdate(product: LauncherProductState) {
  if (busyAction.value || !['update_available', 'download_available'].includes(product.update_status)) return
  if (!product.installed && !product.install_directory) {
    statusText.value = t('chooseInstallLocationThenDownload')
    activeView.value = 'settings'
    return
  }
	downloadProgress.value = { product_id: product.id, phase: 'connecting', bytes: 0, total_bytes: 0 }
	downloadStartedAt.value = 0
  busyAction.value = `prepare:${product.id}`
  try {
    const prepared = await backend.prepareProductUpdate(product.id)
    const index = state.value.products.findIndex(item => item.id === prepared.id)
    if (index >= 0) state.value.products[index] = prepared
    statusText.value = prepared.install_message
    await revealUpdateArea()
  } catch (error) {
    statusText.value = localError(error)
  } finally {
    busyAction.value = ''
  }
}

async function executePreparedUpdate(product: LauncherProductState) {
  if (busyAction.value || !['ready_to_install', 'product_still_running'].includes(product.install_status)) return
  busyAction.value = `install:${product.id}`
  try {
    const started = await backend.executePreparedUpdate(product.id)
    const index = state.value.products.findIndex(item => item.id === started.id)
    if (index >= 0) state.value.products[index] = started
    statusText.value = started.install_message
	if (started.id !== 'meow-launcher' && ['installer_started', 'self_update_installer_started'].includes(started.install_status)) {
	  await waitForInstallReceipt(started.id)
	}
  } catch (error) {
    statusText.value = localError(error)
  } finally {
    busyAction.value = ''
  }
}

async function installProduct(product: LauncherProductState) {
  if (busyAction.value || product.installed || product.release_status !== 'available') return
  downloadProgress.value = null
  downloadStartedAt.value = 0
  busyAction.value = `installflow:${product.id}`
  let current = product
  try {
    if (!['update_available', 'download_available'].includes(current.update_status)) {
      statusText.value = `${current.name} · ${t('updateChecking')}`
      current = await backend.checkProductUpdate(current.id)
      const checkedIndex = state.value.products.findIndex(item => item.id === current.id)
      if (checkedIndex >= 0) state.value.products[checkedIndex] = current
    }
    if (!['update_available', 'download_available'].includes(current.update_status)) {
      statusText.value = updateMessage(current)
      return
    }

    if (!current.install_directory) {
      statusText.value = t('chooseInstallLocationThenDownload')
      activeView.value = 'settings'
      return
    }

    downloadProgress.value = { product_id: current.id, phase: 'connecting', bytes: 0, total_bytes: 0 }
    downloadStartedAt.value = 0
    statusText.value = locale.value === 'zh' ? '正在下载并校验安装包…' : 'Downloading and verifying installer…'
    current = await backend.prepareProductUpdate(current.id)
    const preparedIndex = state.value.products.findIndex(item => item.id === current.id)
    if (preparedIndex >= 0) state.value.products[preparedIndex] = current
    if (current.install_status !== 'ready_to_install') {
      statusText.value = current.install_message
      return
    }

    statusText.value = locale.value === 'zh' ? '安装包已验证，正在启动安装…' : 'Installer verified. Starting installation…'
    current = await backend.executePreparedUpdate(current.id)
    const startedIndex = state.value.products.findIndex(item => item.id === current.id)
    if (startedIndex >= 0) state.value.products[startedIndex] = current
    statusText.value = current.install_message
    if (['installer_started', 'self_update_installer_started'].includes(current.install_status)) {
      await waitForInstallReceipt(current.id, true)
    }
  } catch (error) {
    statusText.value = localError(error)
  } finally {
    busyAction.value = ''
  }
}

async function waitForInstallReceipt(productId: string, openAfterInstall = false) {
  const pending = new Set(['starting', 'installer_started', 'self_update_installer_started'])
  const deadline = Date.now() + 120_000
  while (Date.now() < deadline) {
	await new Promise(resolve => window.setTimeout(resolve, 750))
	const refreshed = await backend.state()
	state.value = refreshed
	const product = refreshed.products.find(item => item.id === productId)
	if (!product || !pending.has(product.install_status)) {
	  if (product) {
		statusText.value = product.install_message
		if (openAfterInstall && product.install_status === 'installed' && product.can_launch) {
		  try {
			await backend.launchProduct(product.id)
			statusText.value = locale.value === 'zh' ? '安装成功，已打开产品' : 'Installed successfully and opened the product'
		  } catch (error) {
			statusText.value = locale.value === 'zh' ? `安装成功，但自动打开失败：${localError(error)}` : `Installed successfully, but could not open automatically: ${localError(error)}`
		  }
		}
	  }
	  return product
	}
  }
  statusText.value = locale.value === 'zh' ? '安装程序等待超时；可稍后点击“打开产品”确认结果' : 'Installer timed out; use Open product to verify later'
  return null
}

async function uninstallCatalogProduct(product: LauncherProductState) {
  if (!product.installed || !product.can_uninstall || busyAction.value) return
  try {
    const plan = await backend.uninstallPlan(product.id)
    if (!plan.installed) return
    const confirmed = window.confirm(`${t('confirmPrefix')} ${plan.product_name}?\n\n${t('confirmBody')}`)
    if (!confirmed) return
    busyAction.value = `uninstall:${plan.product_id}`
    state.value = await backend.uninstallProduct(plan.product_id, plan.product_id)
    statusText.value = `${plan.product_name} ${t('uninstallDone')}`
  } catch (error) {
    statusText.value = localError(error)
  } finally {
    busyAction.value = ''
  }
}

function productAvailability(product: LauncherProductState) {
  if (product.installed) return t('installed')
  if (product.release_status === 'planned') return t('statusPlanned')
	if (product.authorization_status === 'authorized') return t('availableNow')
  return t('availableAfterCheck')
}

function updateStatusLabel(product: LauncherProductState) {
  const labels: Record<LauncherProductState['update_status'], CopyKey> = {
    not_checked: 'statusNotChecked',
    not_installed: 'statusNotInstalled',
    planned: 'statusPlanned',
    check_unavailable: 'statusUnavailable',
    current: 'statusCurrent',
    update_available: 'statusUpdate',
    download_available: 'statusDownload',
    launcher_update_required: 'statusLauncherUpdate',
  }
  return t(labels[product.update_status])
}

function installStatusLabel(product: LauncherProductState) {
  const labels: Record<LauncherProductState['install_status'], CopyKey> = {
    not_started: 'installNotStarted',
    not_available: 'installUnavailable',
    authorization_required: 'installAuthorizationRequired',
    downloading: 'installDownloading',
    download_failed: 'installDownloadFailed',
    ready_to_install: 'readyInstall',
	closing_product: 'installClosingProduct',
	product_still_running: 'installProductStillRunning',
    not_ready: 'installNotReady',
    install_failed: 'installFailed',
    installer_started: 'installStarting',
    self_update_installer_started: 'installStarting',
    location_required: 'installLocationRequired',
    location_selected: 'installLocationSelected',
    installed: 'installInstalled',
  }
  return t(labels[product.install_status])
}

function updateMessage(product: LauncherProductState) {
  const labels: Record<LauncherProductState['update_status'], CopyKey> = {
    not_checked: 'messageNotChecked',
    not_installed: 'messageNotInstalled',
    planned: 'messagePlanned',
    check_unavailable: 'messageUnavailable',
    current: 'messageCurrent',
    update_available: 'messageUpdate',
    download_available: 'messageDownload',
    launcher_update_required: 'messageLauncherUpdate',
  }
  return t(labels[product.update_status])
}

function authorizationLabel(product: LauncherProductState) {
  return product.authorization_status === 'authorized' ? t('authAuthorized') : t('authPending')
}

onMounted(async () => {
  document.documentElement.lang = locale.value === 'zh' ? 'zh-CN' : 'en'
  stopDownloadProgress = EventsOn('launcher-download-progress', receiveDownloadProgress)
  stopUpdateRequest = EventsOn('launcher-update-request', receiveLauncherUpdateRequest)
  await refreshState()
  const product = selectedProduct.value
  if (product && !requestedUpdateHandled.value && product.release_status !== 'planned') {
    await checkProductUpdate(product)
  }
})

onUnmounted(() => {
  stopDownloadProgress?.()
  stopUpdateRequest?.()
})
</script>

<template>
  <div class="window-shell launcher-shell">
    <header class="titlebar">
      <div class="title-brand"><span class="title-dot"></span><span>MEOW LAUNCHER</span><small>{{ state.launcher_build }}</small></div>
      <div class="window-controls">
        <button :aria-label="locale === 'zh' ? '最小化' : 'Minimize'" @click="backend.minimise"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 8.5h10" /></svg></button>
        <button :aria-label="locale === 'zh' ? '最大化' : 'Maximize'" @click="backend.maximise"><svg viewBox="0 0 16 16" aria-hidden="true"><rect x="3.5" y="3.5" width="9" height="9" /></svg></button>
        <button class="close" :aria-label="locale === 'zh' ? '关闭' : 'Close'" @click="backend.close"><svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8" /></svg></button>
      </div>
    </header>

    <section class="app-screen">
      <aside class="sidebar launcher-sidebar">
        <div class="brand-block"><img src="/assets/meow-forge-symbol-reverse-hd.png" alt="Meow symbol" /><div><strong>meow</strong><span>launcher</span></div></div>
        <nav class="side-nav">
          <button :class="{ active: activeView === 'product' }" @click="setView('product')"><span>01</span>{{ t('navProduct') }}</button>
          <button :class="{ active: activeView === 'catalog' }" @click="setView('catalog')"><span>02</span>{{ t('navUninstall') }}</button>
          <button :class="{ active: activeView === 'settings' }" @click="setView('settings')"><span>03</span>{{ t('navSettings') }}</button>
        </nav>
        <div class="side-foot"><span class="live-dot"></span><div><strong>{{ t('hubTitle') }}</strong><small>{{ t('hubSub') }}</small></div></div>
      </aside>

      <main class="main-area">
        <div class="toolbar launcher-toolbar">
          <div><strong>{{ toolbarTitle }}</strong><span>{{ t(activeView === 'product' ? 'productEyebrow' : activeView === 'catalog' ? 'uninstallEyebrow' : 'settingsEyebrow') }}</span></div>
          <div class="toolbar-actions">
            <div class="locale-switch" role="group" aria-label="Language">
              <button :class="{ active: locale === 'zh' }" @click="setLocale('zh')">中文</button>
              <button :class="{ active: locale === 'en' }" @click="setLocale('en')">EN</button>
            </div>
            <button class="refresh-button" :title="t('refresh')" @click="refreshState">↻</button>
          </div>
        </div>

        <div ref="launcherScroll" class="view-scroll launcher-view-scroll">
          <section v-if="activeView === 'product' && selectedProduct" class="view-page launcher-page">
            <div class="page-head product-page-head">
              <h2>{{ t('productTitle') }}</h2>
              <label class="product-picker"><span>{{ t('selectProduct') }}</span><select v-model="selectedProductId"><option v-for="product in state.products" :key="product.id" :value="product.id">{{ product.name }}</option></select></label>
            </div>

            <article class="product-detail-card" :aria-busy="!!busyAction">
              <div class="product-detail-heading">
                <div class="product-title-copy">
                  <span>{{ channelLabel(selectedProduct.channel) }}</span>
                  <h3>{{ selectedProduct.name }}</h3>
                  <p>{{ productDescription(selectedProduct) }}</p>
                </div>
                <strong :class="{ installed: selectedProduct.installed }"><i></i>{{ productAvailability(selectedProduct) }}</strong>
              </div>

              <div class="product-version-summary">
                <div class="version-comparison">
                  <div class="version-value version-local"><small>{{ t('installedVersion') }}</small><strong>{{ localizedVersion(selectedProduct.current_version) }}</strong></div>
                  <span class="version-divider" aria-hidden="true">→</span>
                  <div class="version-value"><small>{{ t('latestVersion') }}</small><strong>{{ localizedVersion(selectedProduct.latest_version || selectedProduct.catalog_version) }}</strong></div>
                </div>
                <div class="version-state">
                  <small>{{ t('updateStatus') }}</small>
                  <strong>{{ updateStatusLabel(selectedProduct) }}</strong>
                  <time v-if="selectedProduct.last_checked_at" :datetime="selectedProduct.last_checked_at">{{ formatDateTime(selectedProduct.last_checked_at) }}</time>
                </div>
                <dl class="product-facts">
                  <div><dt>{{ t('channel') }}</dt><dd>{{ channelLabel(selectedProduct.channel) }}</dd></div>
                  <div v-if="selectedProduct.install_directory" class="product-path"><dt>{{ t('installLocation') }}</dt><dd :title="selectedProduct.install_directory">{{ selectedProduct.install_directory }}</dd></div>
                  <div v-if="selectedProduct.authorization_status !== 'authorized' && (!selectedProduct.installed || selectedProduct.update_status === 'update_available')"><dt>{{ t('authorizationStatus') }}</dt><dd>{{ authorizationLabel(selectedProduct) }}</dd></div>
                  <div v-if="!['not_started', 'installed'].includes(selectedProduct.install_status)"><dt>{{ t('installState') }}</dt><dd>{{ installStatusLabel(selectedProduct) }}</dd></div>
                </dl>
              </div>

			  <div v-if="selectedProduct.release_notes" class="release-note">
				<span>{{ t('releaseNotes') }}</span>
				<p>{{ selectedProduct.release_notes }}</p>
				<time v-if="selectedProduct.published_at" :datetime="selectedProduct.published_at">{{ formatDateTime(selectedProduct.published_at) }}</time>
			  </div>

			  <div v-if="selectedProduct.installed && selectedProduct.id !== 'meow-launcher' && ['ready_to_install', 'product_still_running'].includes(selectedProduct.install_status)" class="boundary-note">
				<span>{{ locale === 'zh' ? '安装前确认' : 'Before installing' }}</span>
				<p>{{ t('closeBeforeInstall') }}</p>
			  </div>
			  <div v-if="selectedProduct.id === 'meow-launcher' && selectedProduct.install_status === 'ready_to_install'" class="boundary-note">
				<span>{{ locale === 'zh' ? 'Launcher 自更新' : 'Launcher self-update' }}</span>
				<p>{{ locale === 'zh' ? '点击后会启动当次下载的一次性安装程序，Launcher 随即退出；安装完成后重新打开 Launcher，不会留下常驻更新器。' : 'A one-shot installer starts and Launcher exits. It reopens after installation and leaves no resident updater behind.' }}</p>
			  </div>

              <div v-if="state.update_history_status === 'error'" class="boundary-note">
                <span>{{ locale === 'zh' ? '安装记录不可用' : 'Install history unavailable' }}</span>
                <p>{{ state.update_history_message }}</p>
              </div>

              <div v-if="selectedProduct.installed && ['ready_to_install', 'product_still_running'].includes(selectedProduct.install_status)" class="download-ready" role="status" aria-live="polite">
                <span>✓</span>
				<div><strong>{{ locale === 'zh' ? '下载与校验完成' : 'Download and verification complete' }}</strong><p>{{ selectedProduct.id === 'meow-launcher' ? (locale === 'zh' ? '一次性安装程序已经准备好。更新时 Launcher 会退出，完成后重新打开。' : 'The one-shot installer is ready. Launcher exits during the update and reopens afterward.') : (locale === 'zh' ? '安装包已经准备好。点击“关闭并更新”后，Launcher 会正常关闭当前产品并只重启一个新版窗口。' : 'The installer is ready. Click Close and update; Launcher will close the current product normally and restart exactly one updated window.') }}</p></div>
              </div>

              <div v-if="(busyAction === `prepare:${selectedProduct.id}` || busyAction === `installflow:${selectedProduct.id}`) && downloadProgress?.product_id === selectedProduct.id" class="operation-progress" role="status" aria-live="polite">
                <div class="download-progress-heading"><span></span><p><strong>{{ downloadProgress?.phase === 'verifying' ? t('downloadVerifying') : downloadProgress?.phase === 'connecting' ? t('downloadConnecting') : t('preparingUpdate') }}</strong>{{ t('downloadSlowHint') }}</p></div>
                <div class="download-progress-track" :class="{ indeterminate: !downloadProgress?.total_bytes }"><i :style="downloadProgress?.total_bytes ? { width: `${downloadPercent}%` } : undefined"></i></div>
                <div class="download-progress-meta"><strong>{{ downloadProgress?.total_bytes ? `${downloadPercent}%` : '—' }}</strong><span>{{ formatBytes(downloadProgress?.bytes || 0) }}<template v-if="downloadProgress?.total_bytes"> / {{ formatBytes(downloadProgress.total_bytes) }}</template></span><span>{{ downloadSpeed() }}</span></div>
              </div>

              <div v-if="selectedProduct.install_status === 'download_failed'" class="download-failure" role="alert">
                <strong>{{ locale === 'zh' ? '下载失败' : 'Download failed' }}</strong>
                <p>{{ selectedProduct.install_message }}</p>
                <code v-if="selectedProduct.error_code">{{ selectedProduct.error_code }}</code>
              </div>

              <div ref="updateActionArea" class="product-detail-actions">
                <button v-if="!selectedProduct.installed && selectedProduct.release_status === 'available'" class="primary install-product-action" :disabled="!!busyAction" @click="installProduct(selectedProduct)">{{ busyAction === `installflow:${selectedProduct.id}` ? (locale === 'zh' ? '正在安装…' : 'Installing…') : (locale === 'zh' ? `安装 ${selectedProduct.name}` : `Install ${selectedProduct.name}`) }}</button>
                <button v-if="selectedProduct.can_launch && selectedProduct.id !== 'meow-launcher'" class="primary open-product-action" :disabled="!!busyAction" @click="launchProduct(selectedProduct)">{{ busyAction === `launch:${selectedProduct.id}` ? t('launching') : (locale === 'zh' ? `打开 ${selectedProduct.name}` : `Open ${selectedProduct.name}`) }}</button>
                <button v-else-if="selectedProduct.release_status === 'planned'" class="secondary" disabled>{{ t('planned') }}</button>
                <button v-if="selectedProduct.installed" class="secondary" :disabled="!!busyAction || selectedProduct.release_status === 'planned'" @click="checkProductUpdate(selectedProduct)">{{ busyAction === `update:${selectedProduct.id}` ? t('checking') : t('checkUpdate') }}</button>
                <button v-if="selectedProduct.installed && ['update_available', 'download_available'].includes(selectedProduct.update_status)" class="primary" :disabled="!!busyAction || ['ready_to_install', 'product_still_running'].includes(selectedProduct.install_status)" @click="prepareProductUpdate(selectedProduct)">{{ busyAction === `prepare:${selectedProduct.id}` ? t('preparingUpdate') : t('prepareUpdate') }}</button>
                <button v-if="selectedProduct.installed && ['ready_to_install', 'product_still_running'].includes(selectedProduct.install_status)" class="primary install-final-action" :disabled="!!busyAction" @click="executePreparedUpdate(selectedProduct)">{{ busyAction === `install:${selectedProduct.id}` ? (selectedProduct.id === 'meow-launcher' ? (locale === 'zh' ? '正在启动一次性安装…' : 'Starting one-shot installer…') : t('startingInstaller')) : (selectedProduct.id === 'meow-launcher' ? (locale === 'zh' ? '更新 Launcher' : 'Update Launcher') : (selectedProduct.install_status === 'product_still_running' ? (locale === 'zh' ? '已关闭，重试更新' : 'Closed it, retry update') : t('installNow'))) }}</button>
              </div>
            </article>

          </section>

          <section v-else-if="activeView === 'catalog'" class="view-page launcher-page catalog-page">
            <div class="page-head">
              <h2>{{ t('uninstallTitle') }}</h2>
            </div>

            <div class="product-catalog" :aria-busy="!!busyAction">
              <article v-for="product in state.products" :key="product.id" class="catalog-card">
                <div class="catalog-card-heading">
                  <div class="catalog-card-identity">
                    <span class="catalog-product-icon" aria-hidden="true">
                      <img v-if="productIconSource(product)" :src="productIconSource(product)" alt="">
                      <span v-else>{{ productIconFallback(product) }}</span>
                    </span>
                    <div class="catalog-card-copy">
                      <h3>{{ product.name }}</h3>
                      <p>{{ productDescription(product) }}</p>
                    </div>
                  </div>
                  <strong :class="{ installed: product.installed }"><i></i>{{ productAvailability(product) }}</strong>
                </div>
                <dl class="catalog-card-facts">
                  <div><dt>{{ t('catalogVersionLabel') }}</dt><dd>{{ localizedVersion(product.current_version || product.latest_version || product.catalog_version) }}</dd></div>
                  <div><dt>{{ t('channel') }}</dt><dd>{{ channelLabel(product.channel) }}</dd></div>
                </dl>
                <div class="catalog-card-actions">
                  <button v-if="product.installed && product.can_uninstall" class="danger-action" :disabled="!!busyAction" @click="uninstallCatalogProduct(product)">{{ busyAction === `uninstall:${product.id}` ? t('uninstalling') : t('catalogUninstall') }}</button>
                  <button v-else-if="!product.installed && product.release_status === 'available'" class="primary" :disabled="!!busyAction" @click="openProductDetail(product, true)">{{ t('catalogDownload') }}</button>
                  <button v-else-if="product.release_status === 'planned'" class="secondary" disabled>{{ t('planned') }}</button>
                  <button v-else class="secondary" :disabled="!!busyAction" @click="openProductDetail(product)">{{ t('catalogDetails') }}</button>
                </div>
              </article>
            </div>
          </section>

          <section v-else class="view-page launcher-page settings-page">
            <div class="page-head"><h2>{{ t('settingsTitle') }}</h2></div>
            <div class="launcher-settings-stack">
              <section class="launcher-setting-primary">
                <div class="launcher-setting-heading">
                  <div>
                    <h3>{{ t('systemLocationTitle') }}</h3>
                    <p>{{ t('systemLocationText') }}</p>
                  </div>
                  <button class="primary" :disabled="!!busyAction" @click="chooseMeowSystemRoot">{{ busyAction === 'system-location' ? t('choosingSystemLocation') : state.meow_system_root ? t('changeSystemLocation') : t('chooseSystemLocation') }}</button>
                </div>
                <div :class="['launcher-setting-location', state.meow_system_status]" aria-live="polite">
                  <i aria-hidden="true"></i>
                  <div>
                    <code>{{ state.meow_system_root || t('systemLocationUnset') }}</code>
                    <small>{{ state.meow_system_message }}</small>
                  </div>
                </div>
              </section>
              <section class="launcher-account-setting">
                <div>
                  <h3>{{ t('accountSectionTitle') }}</h3>
                  <p>{{ t('accountText') }}</p>
                </div>
                <strong>{{ accountStatus }}</strong>
              </section>
            </div>
          </section>
        </div>

        <footer class="statusbar" aria-live="polite"><span class="live-dot"></span><strong>{{ statusDisplay }}</strong><span>{{ t('footer') }} · {{ state.products.length }} {{ t('products') }}</span></footer>
      </main>
    </section>
  </div>
</template>
