# 模板选择授权共享边界

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

selection.go 是组织、团队、密钥普通保存与专用模板绑定共用的授权边界，没有独立 HTTP Module。Selection(h, request, principal, id) 先 TrimSpace；空字符串成功，含义是继承。
非空 ID 从 IAM 读取模板，使用模板真实 organization_id/team_id 构造 authz.Object，然后检查 ActionRouteTemplateRead。缺失返回 ErrNotFound；读取故障封装 InternalError 并记录脱敏错误；不可见的模板不能写入目标对象。
这里校验能否选择模板，目标组织/团队/密钥的写权限由调用处理器另行检查。调用者必须在任何 mutation 前完成两个授权步骤。模板的运行时优先级和整文档规则在 prefs/gateway route_settings，不在此函数。

## 源码职责与入口

### selection.go

公开类型：`Host`.

- [`func Selection(h Host, r *http.Request, p *auth.Principal, id string) error`](selection.go) — Selection checks the selected template before any scope is mutated. Empty means inherit and needs no template read. Ownership must be loaded explicitly: route templates are not resolved by Authorize from their ID alone.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/authz](../../authz/readme_cn.md), [internal/iam](../../iam/readme_cn.md), [internal/logx](../../logx/readme_cn.md).

## 验证与维护入口

当前目录没有直接测试文件；上层集成测试仅证明被执行的链路，不代表所有内部失败分支均已覆盖。

```bash
go test ./internal/gateway/templateauth -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
