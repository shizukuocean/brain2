# Meow 产品统一更新接入指南 v1

这份文档是以后新增 Meow 产品时的首要入口。目标是复用已经在 Brain2 Manager、Dev Helper 和 Meow Launcher 上完成实机验收的更新闭环，不为每个产品重新实现下载器、安装器或授权流程。

底层字段与错误码以 [`update-contract-v1.md`](update-contract-v1.md) 为准；实际发布操作以 [`release-runbook-v1.md`](release-runbook-v1.md) 为准。

## 一句话架构

```text
产品应用：发现更新 + 显示红点/说明 + 精确调起 Launcher
Launcher：授权 + 下载进度 + 校验 + 关闭旧进程 + 安装/回滚 + 原路径重启 + 记录结果
当次完整安装程序：替换文件后退出；不驻留，不成为第四个产品
```

公开“有新版本”和“本次能否安装”必须分开：应用可以先告诉用户确有更新，真正安装时再由 Launcher 核验登录、会员、购买或授权状态。授权不足不能显示成“没有更新”。

## 组件归属

| 能力 | 唯一 owner | 新产品是否重复实现 |
|---|---|---|
| 版本号与公开清单读取 | 产品应用的轻量检查器 | 只接同一 contract，不写下载器 |
| 设置入口红点、版本说明、立即更新按钮 | 产品应用 | 需要按产品 UI 接入 |
| `--update-product <product_id> --return restart` | Launcher 调起适配器 | 产品只传 canonical `product_id` |
| 安装资格、登录、会员、购买状态 | Launcher | 不重复 |
| 下载进度、超时、重试、HTTPS、SHA-256、签名 | Launcher | 不重复 |
| 首次目录选择、后续原路径更新 | Launcher | 不重复 |
| 精确关闭旧进程、失败重试、单实例重启 | Launcher | 不重复 |
| 完整安装包、文件所有权、备份、回滚、回执 | Launcher 的打包/安装事务 | 不重复 |
| Launcher 自更新 | 当次 Release 的一次性完整安装程序 | 不新增常驻更新器 |
| 产品用户数据迁移 | 产品自身另立迁移方案 | 不借更新 MVP 偷渡迁移 |

## 新产品接入清单

### 1. 固定产品身份

- 分配不可复用、不可随展示名改变的 kebab-case `product_id`，例如 `dev-helper`。
- 明确展示名、当前版本来源、主可执行文件相对路径、默认目录名和更新通道。
- `product_id` 必须同时出现在产品构建信息、Launcher 产品目录、发布清单、完整安装包内置清单、安装回执和测试中。
- Brain2 Vault/Runtime 的登记身份不能冒充桌面应用身份；例如 `brain2` 与 `brain2-manager` 必须分开。

### 2. 给产品应用接入轻量检查器

产品应用只需要：

1. 从打包时的 build info 读取当前版本与永久清单 URL。
2. 启动后静默检查一次；失败不强制弹窗，也不能阻塞正常使用。
3. 用户主动点击时允许重新检查。
4. 只比较本产品、当前 channel 的版本，保留 `update_available`、`current`、网络失败、清单非法和产品缺失等独立状态。
5. 发现更新后在设置入口显示与 Meow 系列一致的实心橙色红点，在设置页展示当前版本、线上版本和更新说明。
6. 点击“立即更新”时只精确调起 Launcher，不自行下载安装。

对外行为必须等价于：

```text
Meow Launcher.exe --update-product <product_id> --return restart
```

Launcher 路径从每机 `launcher-location.json` 读取并验证为普通文件；开发环境变量只用于测试。找不到 Launcher 时显示 `LAUNCHER_NOT_FOUND` 和可理解的修复提示。

现有参考实现：

- Brain2 Manager：本仓库 Go 后端与 Manager 前端更新入口。
- Dev Helper：`src/update/contract-v1.js`、`src/integration/launcher-update-v1.js`、`desktop/main.js`。

### 3. 在 Launcher 登记产品

当前 MVP 的产品目录仍由代码模型和前端文案共同组成。接入新产品时至少检查：

- `internal/model/products.go`：产品 ID、名称、描述、版本、安装与启动能力。
- `frontend/launcher/src/App.vue`：产品说明、状态文案和操作按钮；通用更新动作不得复制为产品专用下载逻辑。
- 启动/路径适配器：能从 Launcher 记录的安装状态解析主可执行文件。
- 安装历史与所有权：安装后生成 `.meow-installed-files.json`。产品必须位于所选磁盘的固定 `Meow System` 子目录；规范目录内后续原路径更新，旧目录只在新版成功重启后按有效所有权清单迁移收口。
- 桌面入口：安装事务为当前用户创建或更新以 EXE 内嵌图标显示的 `.lnk`。快捷方式必须指向真实安装入口；目录迁移时跟随新入口更新，失败时应用文件回滚，不得复制第二份 EXE 到桌面。
- 卸载边界：只删除所有权清单登记的应用文件和仍精确指向该入口的 Meow 桌面快捷方式；用户数据、Brain2、旁路文件和同名但属于其他程序的快捷方式默认保留。

如果新产品需要业务授权或 Brain2 项目上下文，那是独立的产品启动契约；不要把它与“是否有更新”或安装授权混在一起。

### 4. 生成完整安装包

每个版本提供一个完整、自包含的 Windows 安装程序，内置：

- canonical `product_id` 与 `version`
- 主可执行文件相对路径
- 全部应用文件
- 每个文件的大小与 SHA-256
- 是否为 Launcher 自更新包

普通产品使用 `meow-bundle`；只有 `meow-launcher` 使用 `meow-bundle-one-shot`。安装事务必须核对调用参数与包内身份，写回带 transaction ID 的结果回执；没有成功回执，Launcher 不得推进已安装版本。

### 5. 加入发布清单与构建门禁

发布条目必须符合 `meow-update-manifest/v1`。Preview 清单使用永久 HTTPS 地址，下载 URL 指向本次不可变 Release 资产。不得把个人 GitHub Token、账号或一次性授权写入应用或清单。

当前 `scripts/build-update-preview.ps1`、`scripts/test-update-preview.ps1` 和 `scripts/audit-update-release.ps1` 仍按前三个产品组织。接入第四个产品前，必须先把这些脚本从“固定三项”改为由受控产品目录驱动，并增加对完整产品集合的断言；不能只往公开清单手工塞一条记录。

### 6. 完成自动与实机验收

每个新产品至少覆盖以下矩阵：

| 路径 | 必须证明 |
|---|---|
| 清单检查 | 新版、已是最新、产品缺失、清单非法、网络失败分别显示 |
| Launcher 缺失 | 产品给出明确修复提示，不崩溃 |
| 授权 | `public` 可直接继续；`entitlement` 的未登录、无资格、服务失败分别显示 |
| 首次安装 | 用户选择目录，完成后可从 Launcher 打开 |
| 后续更新 | 沿用原目录，不再次要求选路径 |
| 下载 | 显示百分比、已下载/总容量、速度和可重试错误 |
| 校验 | SHA-256 不一致和签名不可信时绝不启动安装 |
| 运行中更新 | 先请求目标应用正常退出，只匹配规范化后的完整可执行路径 |
| 替换失败 | 保留旧版本或从备份恢复，Launcher 显示失败原因 |
| 重启 | `return=restart` 后只出现一个新版窗口/一个正常进程树 |
| 用户数据 | 安装、更新、回滚和卸载默认不修改产品用户数据或 Brain2 |
| 桌面快捷方式 | 首装与更新后存在并指向真实 EXE；显示成品内嵌图标；失败回滚；卸载后无断链 Meow 快捷方式 |
| Launcher 自更新 | 等待旧 PID、替换、必要时恢复、重启后一次性安装程序退出；无服务、无计划任务、无常驻更新器 |

自动门禁通过不等于产品发布；还必须取得用户对精确候选和 tag 的明确许可，并在发布后匿名完整下载所有资产重新核验哈希。

## 已验收基线（设备 B，2026-08）

- Brain2 Manager 与 Dev Helper：启动静默检查、设置入口红点、更新说明、精确跳转 Launcher、下载进度、正常关闭旧窗口、原路径替换和单实例/单进程树重启已实机验收。
- Launcher：首次安装、产品安装/打开、安装历史、下载与校验、一次性自更新、重启与非驻留边界已完成本地 E2E 和多轮 Preview 实机更新。
- 公开 Preview：源码仓保持私有；公开下载仓 `shizukuocean/meow-releases` 只保存安装包、哈希与永久通道清单。
- 当前 Preview 采用 HTTPS + SHA-256；Stable 在配置 Authenticode 发布者、受控私钥和时间戳前不得开放。
- 没有接入账号或授权码时使用 `access_policy: public`；以后切换 `entitlement` 不改变产品应用的发现更新逻辑。

## 不得回退的边界

- 不让每个产品维护一套完整下载安装器。
- 不把 Git push 当作产品发布。
- 不把源码仓公开作为下载方案。
- 不把云盘当运行中代码仓库或构建根目录。
- 不在应用内嵌个人 Token。
- 不使用旧 nonce、旧 Agent 身份或旧的一次性安装授权。
- 不静默强制更新。
- 不新增服务、计划任务或长期常驻外部更新器。
- 不因程序更新自动迁移或删除用户数据。

## 真值与维护

- 本文：新产品接入流程与 owner 边界。
- `update-contract-v1.md`：机器可执行字段、校验规则、错误码和安装器语义。
- `release-runbook-v1.md`：B 机构建、门禁、发布、远程复核和 A 机前置条件。
- 源码与测试：最终实现真值；文档与实现冲突时先停止发布，修正文档或代码并重新验收。
- Brain2：保存产品决策、验收事实、跨设备路径和本指南入口，不复制整套源码实现。
