# 三产品更新发布操作手册 v1

> 本手册负责发布操作。新增产品的职责拆分、代码接入点与验收矩阵见 [`meow-product-update-integration-guide-v1.md`](meow-product-update-integration-guide-v1.md)。

这份手册面向当前的公开预览 MVP。用户不需要理解安装器技术或维护另一个更新器；三产品使用完整单文件包，Launcher 的安装程序执行完即退出。

首次取得 Launcher 时，用户直接双击完整包并选择安装磁盘，安装器固定写入 `盘符:\Meow System\Launcher`。其余产品分别固定写入同盘的 `Brain2 Manager` 和 `Helper` 子目录。之后同一产品的更新全部由 Launcher 发起；直接再次运行安装包不会覆盖现有 Launcher。

## 当前发布模型

- 源码仓库可以继续保持私有。
- 更新清单和三个安装包必须放在无需登录、无需个人 Token 即可访问的公开 HTTPS 地址。
- 建议新建独立的公开下载仓库或等价静态下载域名；不要把源码仓库改成公开，也不要把 GitHub Token 写进应用。
- 预览通道允许 HTTPS + SHA-256；稳定通道必须使用 Windows Authenticode 和时间戳。
- 当前没有登录或授权码，因此预览清单使用 `access_policy: public`。以后付费版本改为 `entitlement`，安装时由 Launcher 查询授权端点。

## B 机发布前

`BaseUrl` 指向本次版本的安装包目录，`ManifestUrl` 指向不会随版本变化的公开 Preview 通道清单。两者必须分开：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\stage-update-release.ps1 `
  -BaseUrl "https://github.com/shizukuocean/meow-releases/releases/download/本次版本标签" `
  -ManifestUrl "https://shizukuocean.github.io/meow-releases/channels/preview/manifest-v1.json" `
  -PreviousManagerInstaller "本机已核验的上一版 Manager 完整安装包路径" `
  -PreviousHelperInstaller "本机已核验的上一版 Helper 完整安装包路径" `
  -PreviousLauncherInstaller "本机已核验的上一版 Launcher 完整安装包路径"
```

脚本默认从 `docs/product/release-notes-preview-v1.json` 读取本轮 MVP 的中文更新说明。以后每次发布时，先复制或修改这份可读 JSON，也可用 `-ReleaseNotesPath` 指向另一份说明文件；空说明、非法版本号、内嵌账号或令牌的 URL 会在构建前被拒绝。

脚本会重新构建三个产品、运行 Manager/Helper 安装、Launcher 首次安装和一次性自更新 E2E、验证隔离桌面中的小猫图标快捷方式、执行发布门禁，然后只在 `publish` 目录生成待上传文件。传入两个上一版完整安装包时，它还会先把上一版安装到隔离目录，再用本次候选原路径更新，验证可执行文件确实被替换、快捷方式跟随真实入口、旁路测试用户文件仍保留；两个旧包参数必须成对提供。它不会 commit、push、创建 Release 或上传。

待上传文件只有：

- `manifest-v1.json`
- 三个产品的 `.exe` 完整安装包
- `SHA256SUMS.txt`

## 获得发布许可后

先以默认预检模式确认公开仓、tag、三份安装包和本地清单完全对应；这一步不会上传：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\publish-update-release.ps1 `
  -ReleaseRoot "本次 release_root" `
  -Tag "本次版本标签"
```

取得用户对该 tag 和候选目录的明确发布许可后，才同时传入 `-Publish` 与逐字相同的 `-ConfirmTag`：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\publish-update-release.ps1 `
  -ReleaseRoot "本次 release_root" `
  -Tag "本次版本标签" `
  -Publish `
  -ConfirmTag "本次版本标签"
```

脚本先创建公开 Pre-release 并确认三份安装包可达，再原子更新永久 Preview 清单；随后等待 GitHub Pages 生效，并匿名完整下载三份安装包核对 SHA-256。若远程验收失败，会恢复上一个通道清单，并只清理本次事务刚创建的失败 Release/tag，以便同一候选安全重试；既有 Release 不会被删除。若自动清理也失败，脚本会明确报警且不会假装发布成功。不得修改已经暂存的清单或安装包；任何修改都必须重新构建和审计。

也可以再次单独执行匿名远程验收：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\audit-update-release.ps1 -ReleaseRoot "本次 release_root" -Publish -RemoteManifestUrl "https://shizukuocean.github.io/meow-releases/channels/preview/manifest-v1.json"
```

只有 `remote_verified: true` 才证明清单和全部安装包可被 A 机无 Token 下载且 SHA-256 一致。

## A 机之前必须完成

- 备份并核验 Brain2 数据、项目注册表、用户设置和本地独有内容。
- 保留旧安装，先用隔离目录验证 Launcher 首次安装和一次真实更新。
- 验证回滚后，再制定旧版本卸载、残留清理和恢复步骤。
- A 机操作必须获得用户单独许可，不能与 B 机发布自动合并执行。
