# 本轮公开源码验证记录

验证日期：2026-10-19。本记录只陈述实际执行结果，不等于用户完整产品验收，不包含真实安装、升级或回滚操作。测试使用隔离的 AppData / 临时工作区，不附私有运行数据。

## 同源工程的构建与测试

- Windows / Go 1.26.5 / Node.js；通过公共模块与 npm 依赖完成构建，未使用私人依赖仓库。
- `scripts/prepare-public-build.ps1`：Windows 与 Linux MCP Host 从源码生成；Manager 和 Launcher 两套 Vue / TypeScript / Vite 前端构建通过。生成的二进制、node_modules、dist 不进入公开 Git。
- 隔离环境 `go test ./...`：201 个测试通过，8 个跳过，0 失败；跳过项仍保留在原测试中，不能算通过。
- `go vet ./...`：通过。
- `go test -tags launcher .`：31 通过，1 失败。失败项 `TestOlderSuccessfulInstallRecordDoesNotHidePreparedUpdate` 使用了未创建实际 EXE 的旧路径 fixture，与当前安装记录校验行为不一致；本任务未改功能或将测试改成无条件通过。
- 脱敏设备名称后，相关设备/迁移测试另行复测：6 通过。
- 默认 Go 套件初次运行因 `proxy.golang.org` 直连超时未完成；使用公开镜像下载并继续保留 `go.sum` 校验后，在隔离环境重跑得到以上结果。

Brain2 和 Launcher 两份公开快照的共享产品源码完全同源。上述套件在 Launcher 公开副本上实际执行，是共享源码的验证证据，不虚报成两次独立测试。

## 可直接查看的证据

- [Brain2 模块与测试](../internal/brain2)
- [Agent 连接与测试](../internal/agentconnect)
- [更新事务与测试](../internal/updateinstall)
- [Launcher 专用测试](../launcher_app_test.go)

## 未验证范围

没有执行生产产品替换、自更新、真实回滚、签名证书配置、账号服务上线或全部人工桌面交互。源内默认版本与发布参数版本存在区别，不能仅凭旧 README 把预览构建当作正式商用状态。
