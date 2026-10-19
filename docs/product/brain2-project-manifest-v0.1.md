# Brain2 Project Manifest v0.1

Project Manifest 是 Brain2 项目的“身份证”。它把人类可读的治理真值和下游产品可读的机器索引连接起来。

Launcher 同时维护设备级 `Meow Product Registry`，让独立运行的 Blueprint/Protector 先发现 Brain2 实例，再只读定位 `projects.json`。默认写入当前 Windows 用户的 `%LOCALAPPDATA%\MeowLauncher\runtime\brain2-registry.json`；测试或受管部署可通过绝对路径环境变量覆盖。

## 真值关系

```text
02 项目/<项目名>/项目资产登记.md
= 项目路径、资产角色和代码真值的治理入口

.brain2/projects.json
= Launcher、Meow Blueprint、Meow Protector 与健康检查使用的机器索引
```

两者不一致时，健康检查返回 `needs_attention`；不得静默选择其中一份。核心 Brain2 文件损坏时仍返回 `critical`。

## 版本

- Project Manifest：`0.1`
- Project Registry：`0.3`
- `projects.json` Schema：`packages/schemas/brain2-projects.schema.json`
- Meow Product Registry：`0.1`，Schema 为 `packages/schemas/meow-product-registry.schema.json`
- 旧 `projects.json` `0.1` / `0.2` 可继续读取，不自动猜测或批量补写旧项目路径。

Registry `0.3` 增加 `entity_kind`、`parent_project_id`、`lifecycle_stage`、`repository_url` 与 `default_branch`。GitHub 仓库是可跨设备复用的代码真值；各设备本地工作树继续作为带 `device_id` 的项目资产登记。

## 首次项目建档闭环

默认流程不要求用户记住项目路径：

1. Launcher 生成 Brain2-first 项目发现任务；Agent 从 Brain2、GitHub 交接和已有登记沿证据核验项目候选。
2. 用户只选择“加入建档”“归入已有项目”或“忽略”，不负责填写技术字段。
3. “加入建档”把候选置为 `selected_for_archiving`，不会提前写进项目 Registry。
4. Launcher 为该候选签发一次性 Archive Task，绑定 Brain2 实例、候选、Agent、源路径和 nonce。
5. Agent 调查对象类型、生命周期、Brain2 入口、GitHub 仓库、默认分支和各设备路径，通过 `brain2_submit_project_archive` 提交。
6. MCP Host 只在 Schema、权限范围、Manifest 与 Registry 一致性通过后写出 `verified` receipt；此时项目才算完成建档。

目录扫描仍作为证据链断裂或路径修复的兜底，不是默认入口。Launcher 不做项目语义判断，Agent 也不能绕过一次性任务直接修改 `.brain2/projects.json`。

## 新项目行为

Launcher 新建内部项目或挂载外部项目时同时：

1. 创建项目 README；
2. 创建 `项目资产登记.md`；
3. 在 `.brain2/projects.json` 写入 `manifest_path`、身份状态、路径状态、真值状态和资产列表；
4. 将下游产品连接门固定为 `user_confirmation_required`。

`registered` 与 `verified` 只代表路径元数据已登记且创建时可访问，不代表 Blueprint 可以修改项目，也不代表 Protector 可以直接扫描。两个产品仍需让用户确认项目和范围。

## V0.1 最小资产

- 内部项目：登记 Brain2 内的项目根目录，路径类型为 `brain2_relative`。
- 外部项目：登记用户明确选择的源目录，路径类型为 `absolute`，不复制、不移动源文件。
- 初始能力仅声明 `project_manifest` 与 `path_provider`，不提前声明具体项目已经完全兼容 Blueprint 或 Protector。

## 旧项目兼容

旧项目缺少 `manifest_path` 时：

- 仍显示在项目列表；
- 健康检查提示“待补身份”；
- Launcher 不根据日志、README 或历史交接自动猜测代码真值；
- 后续需要独立的“确认并补登记”入口。
