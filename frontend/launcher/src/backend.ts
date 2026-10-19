export interface LauncherProductState {
  id: string
  name: string
  description: string
  release_status: 'available' | 'planned'
  catalog_version: string
  current_version?: string
  channel: string
  installed: boolean
  executable_path?: string
  can_launch: boolean
  update_status: 'not_checked' | 'not_installed' | 'planned' | 'check_unavailable' | 'current' | 'update_available' | 'download_available' | 'launcher_update_required'
  update_message: string
  latest_version?: string
  last_checked_at?: string
  can_uninstall: boolean
  release_notes?: string
  published_at?: string
  authorization_status: 'authorized' | 'not_authenticated' | 'denied' | 'unavailable'
  authorization_message: string
  install_status: 'not_started' | 'not_available' | 'authorization_required' | 'downloading' | 'download_failed' | 'ready_to_install' | 'closing_product' | 'product_still_running' | 'not_ready' | 'install_failed' | 'installer_started' | 'self_update_installer_started' | 'location_required' | 'location_selected' | 'installed'
  install_message: string
  downloaded_path?: string
  install_directory?: string
  error_code?: string
}

export interface ProductUninstallPlan {
  product_id: string
  product_name: string
  installed: boolean
  current_version?: string
  application_path?: string
  remove_items: string[]
  preserve_items: string[]
  cleanup_available: boolean
}

export interface LauncherSurfaceState {
  products: LauncherProductState[]
  authenticated: boolean
  account_status: string
  meow_system_root?: string
  meow_system_status: 'ready' | 'required' | 'error'
  meow_system_message: string
  launcher_build: string
  download_service: 'planned' | 'ready'
  requested_product_id?: string
  requested_return?: 'restart'
  update_history_status: 'ready' | 'error'
  update_history_message: string
}

interface LauncherBridge {
  GetState(): Promise<LauncherSurfaceState>
  CheckProductUpdate(productID: string): Promise<LauncherProductState>
  PrepareProductUpdate(productID: string): Promise<LauncherProductState>
  ExecutePreparedUpdate(productID: string): Promise<LauncherProductState>
  ChooseProductInstallDirectory(productID: string): Promise<LauncherProductState>
  ChooseMeowSystemRoot(): Promise<LauncherSurfaceState>
  GetUninstallPlan(productID: string): Promise<ProductUninstallPlan>
  UninstallProduct(productID: string, confirmation: string): Promise<LauncherSurfaceState>
  LaunchProduct(productID: string): Promise<void>
  WindowMinimise(): Promise<void>
  WindowToggleMaximise(): Promise<void>
  WindowClose(): Promise<void>
}

declare global {
  interface Window {
    go?: { main?: { LauncherApp?: LauncherBridge } }
  }
}

function bridge(): LauncherBridge {
  const app = window.go?.main?.LauncherApp
  if (!app) throw new Error('当前页面不在 Meow Launcher 桌面运行环境中')
  return app
}

export const backend = {
  state: () => bridge().GetState(),
  checkProductUpdate: (productID: string) => bridge().CheckProductUpdate(productID),
  prepareProductUpdate: (productID: string) => bridge().PrepareProductUpdate(productID),
  executePreparedUpdate: (productID: string) => bridge().ExecutePreparedUpdate(productID),
  chooseProductInstallDirectory: (productID: string) => bridge().ChooseProductInstallDirectory(productID),
  chooseMeowSystemRoot: () => bridge().ChooseMeowSystemRoot(),
  uninstallPlan: (productID: string) => bridge().GetUninstallPlan(productID),
  uninstallProduct: (productID: string, confirmation: string) => bridge().UninstallProduct(productID, confirmation),
  launchProduct: (productID: string) => bridge().LaunchProduct(productID),
  minimise: () => bridge().WindowMinimise(),
  maximise: () => bridge().WindowToggleMaximise(),
  close: () => bridge().WindowClose(),
}

export function errorMessage(error: unknown): string {
  const raw = error instanceof Error ? error.message : typeof error === 'string' ? error : ''
  return raw || '操作没有完成，请检查安装状态后重试。'
}
