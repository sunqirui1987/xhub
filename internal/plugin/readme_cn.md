# 上游调用扩展注册

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

registry.go 定义 Extension、Call 和 Decision。Extension.Name 与 BeforeUpstream 是扩展契约；Call 包含 Op/Model/Path，Decision 可拒绝并设置 Status/Code/Message/Header。Registry 的 Register 拒绝 nil、空名和重复名，Names 返回副本。
Run 按登记顺序调用扩展，第一条拒绝终止后续执行，先前累积的头按规则保留。Invoke 按确切名称查找。扩展在上游前执行，不能绕过此前的身份、预算和模型权限。
这不是任意动态脚本加载器，也没有 HTTP 安装接口。扩展应避免不可回滚的副作用或额外泄露凭据；拒绝响应必须保持目标协议错误语义。测试覆盖登记、顺序、头合并、查找失败和首次拒绝。

## 源码职责与入口

### registry.go

公开类型：`Call`, `Decision`, `Extension`, `Registry`.

- [`func New() *Registry`](registry.go) — New returns an empty registry. Run allows the call when nothing is registered.
- [`func (r *Registry) Register(ext Extension) error`](registry.go) — Register appends an extension. An empty or duplicate name returns an error and leaves the existing order unchanged.
- [`func (r *Registry) Names() []string`](registry.go) — Names returns a copy of the registered names in order. Changing the slice does not change the registry.
- [`func (r *Registry) Invoke(name string, call Call) (Decision, error)`](registry.go) — Invoke runs the extension registered under name. A missing name returns an error and does not call any other extension.
- [`func (r *Registry) Run(call Call) Decision`](registry.go) — Run calls every extension in registration order. The first refusal stops the rest and keeps headers already set. An empty registry returns a zero Decision so the data plane continues.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [registry_test.go](registry_test.go) | `TestRegistryZeroValueRegistersWithStableName`, `TestRegistryRunMergesHeadersAndStopsAtRefusal` |

```bash
go test ./internal/plugin -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
