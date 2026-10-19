# Brain2 Manager 分离边界 v0.7

## 目标

把 Brain2 功能面板从 Meow Launcher 的桌面进程中分离，同时保持 Brain2 现有功能、页面结构、CSS 视觉系统、用户数据位置和跨产品联动契约。

本次是承载边界迁移，不是 Brain2 功能重构。

## 运行边界

```text
Meow Launcher.exe
├── 用户账号验证
├── 全部产品目录
├── 产品下载、安装、更新与启动
└── 卸载、备份与清理

Brain2 Manager.exe
├── 创建 / 接入本机 Brain2 仓库
├── 原 Brain2 Vue 页面与 CSS
├── 项目与设备管理
├── Agent / MCP 接入
├── 健康检查与连接器更新
└── Helper / Protector 短时产品授权
```

两个进程通过稳定的本机产品与 Brain2 契约协作：

- LocalAppData 中的 Launcher 设置；
- `brain2-registry.json` 总 Registry；
- 当前 Brain2 根目录与 `.brain2` 机器层数据；
- `brain2-product-launch/v1` 协议与历史兼容字段。

两个进程不共享 WebView2 缓存：

- Launcher：`webview2-launcher`；
- Brain2 Manager：`webview2-brain2-manager`。

这使两个窗口可以同时运行，同时避免改变任何 Brain2 或产品身份。

## 兼容不变量

- `brain2-product-launch/v1` 名称、任务字段、回执字段和 scope 不变。
- Brain2 project registry schema 保持 `0.3`。
- Helper scope 保持 `metadata/read/candidate_write`。
- Protector scope 保持 `metadata/read/scan`。
- 逻辑设备 A/B、本机路径选择、路径指纹和短时授权校验不变。
- MCP Host、Skill、连接任务、握手和总 Registry 路径不变。
- 历史 `launcher-*` 标识作为兼容协议值保留，不进行数据迁移或改写。

## Launcher 产品目录边界补充

Launcher 不是 Brain2 Manager 的专属安装器。Brain2 Manager 只是 Shizuku 产品目录中的一个产品；登录后 Launcher 应展示并允许下载账号可见的全部现有产品。资格、已购买、试用或受邀状态属于后续独立授权层，不得反向把 Launcher 收窄为某个产品的附属入口。

当前本地预览先冻结统一产品目录和生命周期界面，不伪造尚未接入的在线登录、下载源、版本清单、签名或购买资格结果。Brain2 仓库的创建、接入、项目、Agent、健康和联动界面均归 Brain2 Manager。

Launcher 产品详情页使用 dropdown 在完整产品目录间切换，统一展示本机版本、目录版本、发布通道和产品应用更新检测。卸载使用显式产品安装清单，只移除产品应用文件；用户内容、Brain2 记忆库、备份和账户记录默认保留，数据清理必须走独立确认与恢复策略。

Brain2 Manager 原更新中心不得整体删除：它维护的是 Brain2 Skill、MCP Host、Agent 配置、受控哈希、备份与回滚，属于 Brain2 内部运行环境。Brain2 Manager 应用本体的版本检测、下载和更新才归 Launcher。

## 构建与验证

```powershell
go test ./...
go test -tags launcher .
npm.cmd run build
npm.cmd run build:launcher
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build-split-windows.ps1
```

设备 B 真实 Brain2 往返验证：

```powershell
$env:MEOW_BRAIN2_INTEGRATION_ROOT='D:\Brain2'
$env:MEOW_BRAIN2_INTEGRATION_DEVICE='B'
$env:MEOW_DEV_HELPER_ROOT='C:\Example\DevHelper'
$env:MEOW_PROTECTOR_DEV_ROOT='C:\Users\example-user\Documents\protector-spike1'
go test ./internal/productlaunch -run TestCurrentBrain2ProjectsRoundTripToHelperAndProtector -count=1 -v
```

验收要求：Launcher 与 Brain2 Manager 可同时运行；Manager 可读取当前工作区、项目和 Agent 状态；真实 Helper 与 Protector 往返测试通过。

## 2026-07-23 本轮实证与交接

- 分支：`codex/separate-brain2-manager`，基于现有 `codex/brain2-project-archive-workflow` 工作线继续，不覆盖此前设备资产修复提交。
- 双产物构建通过：Launcher 与 Manager 均生成在 `dist/`，并通过同目录伴随文件规则连接。
- 实机并行启动通过：旧 Launcher、分离后的 Launcher 与 Brain2 Manager 使用互不冲突的 WebView2 目录。
- Manager 读取设备 B 当前 `D:\Brain2` 成功；验收时显示 6 个项目和 1 个已验证 Agent。
- Go 全量测试、Go vet、两个 Vue / TypeScript 构建和 Launcher build-tag 边界测试通过。
- 设备 B 当前 Brain2 → Dev Helper → Meow Protector 的真实 `brain2-product-launch/v1` 往返测试通过。
- 当前 Codex 会话未获得 Launcher 签发的 Brain2 `connection_id / nonce / agent_id`，因此没有绕过握手写入 Brain2 项目进度；本文件作为下一次 Launcher / Agent 正式接入时的写回依据。
