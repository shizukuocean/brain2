# Meow Update Contract v1

> 新 Meow 产品接入前先读 [`meow-product-update-integration-guide-v1.md`](meow-product-update-integration-guide-v1.md)；本文只定义底层清单、状态、安装器和安全边界。

## 边界

`meow-update-manifest/v1` 是 Dev Helper、Brain2 Manager 与 Meow Launcher 共同读取的公开发布清单。应用只用它判断“是否确有新版本”；账号、会员、购买状态和本次安装权限由 Launcher 在执行安装前单独核验。授权不足不得被翻译成“没有更新”。

本阶段已实现清单读取、校验、版本判断、Launcher 精确产品入口、受控下载和本地安装事务测试闭环，并建立公开 Preview 下载通道；仍不迁移用户数据。

## 产品身份

| product_id | 产品 | 应用本体更新 owner |
|---|---|---|
| `dev-helper` | Dev Helper | Meow Launcher |
| `brain2-manager` | Brain2 Manager | Meow Launcher |
| `meow-launcher` | Meow Launcher | 当次 Release 自带的一次性完整安装程序 |

`brain2` 继续只表示 Brain2 Vault / Runtime 产品登记，不再用作 Brain2 Manager 应用本体的更新身份。

## 清单字段

顶层字段：

- `schema_version`: 固定为 `meow-update-manifest/v1`。
- `manifest_url`: 应用内固定写入并持续读取的永久 HTTPS 通道地址；不得指向某一次版本的 Release 资产地址，也不得内嵌账号或令牌。
- `generated_at`: RFC3339。
- `products`: 每个 `product_id + channel` 最多一条。

产品发布字段：

- `product_id`, `channel`, `version`, `published_at`, `release_notes`
- `download_url`: 生产清单必须是 HTTPS。
- `access_policy`: `public` 表示公开版本当前无需登录；`entitlement` 表示安装前必须核验会员、购买或安装资格。无论哪种策略，安装动作都由 Launcher 统一执行。
- `installer_type`: 对用户统一表现为普通 Windows 安装程序。当前完整载荷包使用 `meow-bundle`；Launcher 自更新使用当次下载后即退出的 `meow-bundle-one-shot`。旧 `nsis` 类型只保留兼容入口。
- `install_scope`: `per-user` 或 `per-machine`；由签名发布清单决定，Launcher 不猜测。
- `install_directory_name`: 发布清单兼容字段，仍须是安全的单层目录名，但不能改变本机安装真值。Launcher 按产品身份固定解析到所选磁盘的 `Meow System` 目录。
- `executable_relative_path`: 安装目录内的相对 `.exe` 路径；禁止绝对路径和 `..` 越界。
- `sha256`
- `signature`: `method`, `required`, `publisher`。本地/预览测试可只校验 HTTPS 与 SHA-256；`stable` 正式渠道必须要求 Windows Authenticode 签名。
- `minimum_launcher_version`, `minimum_supported_version`
- `mandatory`: 本阶段只展示，不做静默强制更新。
- `data_migration_version`: 本阶段为 `none`；不得借更新系统迁移 Brain2 用户数据。
- `rollback`: `strategy`, `previous_version_supported`, `backup_required`。

## 状态与失败码

- `update_available`, `current`
- `source_not_configured` / `UPDATE_SOURCE_NOT_CONFIGURED`
- `check_failed` / `UPDATE_CHECK_FAILED` 或 `UPDATE_SOURCE_INSECURE`
- `manifest_invalid` / `UPDATE_MANIFEST_INVALID`
- `product_not_found` / `UPDATE_PRODUCT_NOT_FOUND`
- Launcher 缺失：`LAUNCHER_NOT_FOUND`
- 首次安装未选择目录：`INSTALL_LOCATION_REQUIRED`
- 安装阶段预留：`AUTHORIZATION_REQUIRED`, `AUTHORIZATION_DENIED`, `HASH_MISMATCH`, `SIGNATURE_INVALID`, `INSTALL_FAILED`, `ROLLBACK_FAILED`

网络、清单、产品缺失和授权失败都必须独立呈现，不能降级成“当前已是最新”。

## 调起语义

对外行为等价于：

```text
meowlauncher://update?product=brain2-manager&return=restart
meowlauncher://update?product=dev-helper&return=restart
```

本地 MVP 先用同等语义的命令行：

```text
Meow Launcher.exe --update-product brain2-manager --return restart
Meow Launcher.exe --update-product dev-helper --return restart
```

Launcher 必须直接打开对应产品详情，而不是首页。

## 发布源审计结论

- 只读 GitHub API 已复核：`shizukuocean/dev-helper` 与 `shizukuocean/meow-launcher` 保持私有；源码不因产品更新公开。
- 独立的 `shizukuocean/meow-releases` 是公开下载仓。GitHub Pages 从 `main` 根目录发布永久 Preview 清单：`https://shizukuocean.github.io/meow-releases/channels/preview/manifest-v1.json`；版本安装包来自该公开仓的 GitHub Pre-release。
- 未登录的 HTTPS 请求已能读取永久清单和公开安装包；应用不内嵌 GitHub Token。公开“有更新”信息与未来会员授权继续分离。
- 当前 Preview 使用 `access_policy: public`，因为产品尚未接入账号或授权码。未来受保护版本改为 `entitlement` 时，只配置 Launcher 的授权服务端点和短期会话，不把个人 GitHub 凭据当作产品授权。
- 本地 `testdata/update/manifest-v1.json` 和 `MEOW_UPDATE_MANIFEST=<绝对路径>` 仍只用于隔离测试，不作为运行中生产源。

## Launcher 自更新

生产实现固定为一次性安装程序：Launcher 下载完整安装程序，校验 HTTPS、SHA-256 与 Authenticode，启动安装程序后主动退出；安装程序替换/恢复并重启 Launcher，随后退出。安装程序不驻留、不形成第四个产品或循环更新器。

## 本地 MVP 实现状态（2026-08-11）

Launcher 当前代码已经实现并测试以下边界：

- 公开清单检查与安装资格分离。`public` 版本无需登录也可继续；未来切换为 `entitlement` 后，没有登录会话、资格不足和授权服务不可用会分别呈现。
- 受保护版本每次“准备更新”都请求新的安装授权编号；执行安装前再次核验，不读取旧 nonce、旧 Helper 身份或旧的一次性授权。
- 安装包只从 HTTPS 下载，设置流式大小上限，写入临时文件后核验 SHA-256 与 Windows Authenticode 发布者，再发布为可安装文件。
- 启动安装程序前重新核验文件哈希与签名，避免下载后被替换。
- 普通产品安装器接收明确的 product/version/return/target 参数；Launcher 自更新只接受 one-shot 类型，把当前 Launcher PID 交给当次安装程序，并且仅在安装程序成功启动后退出。
- 每个 `meow-bundle` 是单文件完整安装程序：内置产品清单、全部应用文件、逐文件大小和 SHA-256。运行时先验证调用身份与内置载荷，再生成目标机安装事务，不依赖常驻外部更新器或可被替换的旁路计划文件。
- 一次性文件替换事务支持等待目标进程、同盘备份、逐文件原子替换、中途失败逆序恢复、安装回执和可选重启；它不是常驻更新器。
- Launcher 在启动安装器前生成随机事务号并原子写入待处理记录；安装器回执必须匹配 product/version/transaction/target，启动后 Launcher 才会推进已安装版本。失败回执保留旧版本，缺失回执保持待核验。
- 首次安装由用户选择安装磁盘，目录结构固定为 `盘符:\Meow System\Launcher`、`盘符:\Meow System\Brain2 Manager` 和 `盘符:\Meow System\Helper`。清单只决定目录内可执行文件相对路径，不能另造产品目录。
- 已安装在 `盘符:\launcher`、`盘符:\manager`、`盘符:\helper` 等旧目录的产品，下次成功更新时写入同盘规范目录。只有新路径安装成功、回执身份匹配且新版已经重启后，Launcher 才能依据旧路径的有效 `.meow-installed-files.json` 清理已登记程序文件；安装或重启失败时旧目录保持不变。

当前代码不会把“安装包已验证”误报为“安装完成”。最终已安装版本和结果必须由正式安装器回执确认。

## 正式安装器必须遵守的启动语义

普通产品安装程序：

```text
installer.exe --meow-product <product_id> --meow-version <version> --meow-transaction <random-id> --meow-receipt <absolute-path> --meow-return restart --meow-target-executable <absolute-path>
```

Launcher 一次性安装程序：

```text
installer.exe --meow-product meow-launcher --meow-version <version> --meow-transaction <random-id> --meow-receipt <absolute-path> --meow-return restart --meow-target-executable <absolute-path> --meow-wait-pid <pid> --meow-restart-launcher
```

参数是 Launcher 与当次 Release 安装程序之间的契约，不代表安装器是独立产品。正式模板还必须把最终回执写入 Launcher 管理的更新记录目录；只有回执状态为成功，Launcher 才能推进已安装版本。

## 已确定的安装体验与生产前提

现有构建现场已经能生成并测试三产品完整 Preview 安装器；候选只有经过明确发布许可、公开 Release/Pages 写入和匿名远程全量复核后，才视为完成当次线上发布：

- `scripts/build-update-preview.ps1` 会构建 Manager、Wails/WebView2 + SEA 后端的完整 Helper、Launcher 和三份单文件安装包，并生成带真实产物 SHA-256 的 v1 清单。Helper 的 Electron 包只保留为迁移回归基线，不再进入新 Release 载荷。
- `scripts/test-update-preview.ps1` 已在隔离目录实际验证 Manager/Helper 安装，以及 Launcher 等待旧进程、替换、回执和重启的非驻留自更新链路；`scripts/test-update-in-place.ps1` 还必须使用线上上一版三产品完整安装包，验证 Manager/Helper 原路径替换和旧 Launcher 跨目录迁入 `Meow System\Launcher`、新版重启后所有权清理、旁路文件保留与快捷方式改指向。
- `scripts/audit-update-release.ps1` 复核三产品身份、HTTPS URL、安装器类型、产物 SHA-256 与稳定版 Authenticode；加 `-Publish` 时会拒绝 `.invalid` 占位域名，作为外部发布前的硬门禁。
- `scripts/stage-update-release.ps1` 分开接收本次安装包目录 `BaseUrl` 与永久通道 `ManifestUrl`，并拒绝把固定版本 Release 地址当作通道地址；脚本重建并完成 E2E，只生成待上传的扁平 `publish` 目录，不调用 Git、GitHub 或任何上传 API，上传后再由审计脚本匿名远程下载复核。
- Preview 安装、打开和卸载入口继续由 Launcher 统一提供。每次成功安装或更新都会为当前用户创建或刷新桌面 `.lnk`，直接指向真实安装入口并使用 EXE 内嵌的小猫图标；快捷方式创建属于安装提交步骤，失败时应用文件恢复到安装前状态。卸载只移除仍精确指向该应用入口的 Meow 快捷方式，不覆盖或删除同名的其他用户入口。当前仍不额外写 Windows“应用和功能”或开始菜单，也不引入常驻更新服务、Windows 服务或计划任务。
- 每个 bundle 安装后写入 `.meow-installed-files.json` 所有权清单；Launcher 卸载只移除清单登记的应用文件和清单本身，保留未登记文件、用户内容与 Brain2 数据。旧 NSIS 模板的递归卸载仍不得直接复用。
- Launcher 启动时把自身绝对路径原子记录到每机 runtime；Manager/Helper 跨安装目录读取该记录，拒绝缺失、目录或符号链接目标。
- Preview 当前以公开 HTTPS + SHA-256 验收，`signature.required=false`。Stable 仍不得沿用此策略；正式 Authenticode 证书的 subject/publisher、时间戳服务、签名工具和受控私钥位置尚未确定。

产品选择已经确定：三个产品都提供普通 Windows 完整安装程序。首次安装由用户选择磁盘并进入固定 `Meow System` 布局；规范目录中的后续更新原路径替换，旧布局则通过一次成功更新安全迁移。Launcher 自更新仍使用该次 Release 自带、执行完即退出的一次性完整安装程序，不新增常驻更新器。

Launcher 的同一份完整包在没有内部更新参数时进入首次安装模式：用户选择目录、确认目标、安装并启动 Launcher，然后安装程序退出。目标位置已存在 Launcher 时首次安装模式拒绝覆盖，必须回到已安装 Launcher 发起带准确旧进程 PID 的一次性自更新。Launcher 驱动的更新不弹安装器阻塞窗口，失败通过回执返回 Launcher 显示。

安装工具的内部名称不需要用户理解，也不会成为需要单独维护的产品。Preview MVP 当前剩余门槛是：真实运行中 Helper 原路径更新验收，以及获得明确许可后发布本次精确候选并完成匿名远程全量复核。Windows 系统入口和 Authenticode 属于后续 Stable 增强；正式 HTTPS Preview 源已经建立。
