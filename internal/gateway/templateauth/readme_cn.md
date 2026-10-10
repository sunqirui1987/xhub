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

[selection_test.go](selection_test.go) 覆盖空白继承、平台/组织/团队真实归属、授权拒绝、模板缺失和存储故障关闭授权。归属测试使用真实 PostgreSQL 专用 schema，测试结束删除，属于服务边界验证；空输入和取消上下文属于确定性单元验证。真实 HTTP 绑定和浏览器流程见[验证矩阵](../../../docs/development/product-fixes-20261010.md)。

```bash
go test ./internal/gateway/templateauth -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
