# Brain2 Product Launch v1

`brain2-product-launch/v1` 是 Brain2 Manager 将 Brain2 当前已确认项目短时交付给 Dev Helper 或 Meow Protector 的本地契约。契约名和字段保持不变，以兼容现有产品适配器。

## 所有者

- Brain2 Manager 任务生成与回执校验：`internal/productlaunch/`
- Helper 薄适配器：`src/integration/brain2-product-launch-v1.js`
- Protector 薄适配器：`desktop/integration/brain2-product-launch/v1/`

Brain2 继续拥有项目身份和治理入口；产品数据仍由各产品拥有。该契约不把 Helper/Protector 数据库复制到 Brain2，也不授权修改用户 Markdown。

## 当前身份

任务必须绑定：

- Brain2 project registry schema `0.3`；
- `workspace_id`；
- 逻辑设备 `device_id`（`A` / `B`），而不是旧机器名；
- 项目 ID、名称、对象类型、生命周期、Brain2 入口；
- 本机已验证资产、短时路径快照和路径指纹；
- 可选 GitHub 仓库与默认分支；
- 产品、版本、scope、有效期和撤销状态。

产品回执必须回显 contract、registry schema、逻辑设备、项目、产品版本、路径指纹和实际接受的 scope。Brain2 Manager 逐项比对后才显示接入成功。

## 设备与权限

- 当前查看设备不是本机时，Brain2 Manager 拒绝启动产品。
- Brain2 Manager 优先选择与本机逻辑设备或机器名匹配的已验证绝对路径资产；旧登记只在路径与当前 `source_path` 一致且本机真实存在时兼容。
- Helper scope 固定为 `metadata / read / candidate_write`。
- Protector scope 固定为 `metadata / read / scan`；项目清单只建立选择会话，用户仍需在 Protector 内逐项目确认。
- 上述 scope 均不包含 repair、发布或生产部署。

## 兼容与验证

旧 `0.1.0-fixture` 与 `0.1.0-catalog-fixture` 只在产品适配器中保留读取兼容；Brain2 Manager 新任务只生成 v1 契约。历史字段值（例如 `launcher-user-confirmed`）继续保留，不因界面分离而改写。

本机真实 Brain2 往返验证：

```powershell
$env:MEOW_BRAIN2_INTEGRATION_ROOT='E:\Brain2'
$env:MEOW_BRAIN2_INTEGRATION_DEVICE='A'
$env:MEOW_DEV_HELPER_ROOT='C:\Example\DevHelper'
$env:MEOW_PROTECTOR_DEV_ROOT='E:\MeowProtector\protector-spike1'
go test ./internal/productlaunch -run TestCurrentBrain2ProjectsRoundTripToHelperAndProtector -count=1 -v
```

该测试读取 Brain2 当前项目 registry，使用临时产品运行目录完成 Helper 握手与 Protector 项目清单握手；不会运行 Protector 扫描，也不会修改项目源码或 Brain2 内容。
