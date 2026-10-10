# 平台设置与请求级路由文档

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

settings.go 与 page.go 管理持久配置及页面输出；`Overlay` 按顶层键用数据库覆盖 YAML，保留 null、列表、零和 false；`MergePatch` 递归合并 map、替换列表。
`route_settings.go` 是 internal/router 的类型和函数别名。`Resolve(lookup, chain...)` 按 key→team→organization 选择首份完整模板；无绑定或选中模板缺失时使用 builtin 模型管理默认分配，缺失模板不会继续选择祖先。网关读取存储故障使请求失败。接口不接受 platform 文档，不逐字段拼接模板。
模型规则、allocations、retry_policy 和 fallback 执行契约见 [internal/router](../../router/readme_cn.md)。默认策略 traffic-split，max_attempts 是每部署总尝试次数；不支持执行的字段在保存时拒绝。

## 源码职责与入口

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### host.go

公开类型：`Host`.

内部实现和协议边界见 [host.go](host.go)。

### merge.go

- [`func Overlay(base, db map[string]any) map[string]any`](merge.go) — Overlay copies the YAML base and then writes every database key on top. A database value wins even when it is a list or null. Keys absent from the database stay as they were.
- [`func MergePatch(current, patch map[string]any) map[string]any`](merge.go) — MergePatch folds a partial update into the current document. Nested objects are merged, while lists and scalars replace the previous value. Keys that the patch does not mention stay.

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module reads and writes router settings and general settings.

### page.go

- [`func Page(s Host, w http.ResponseWriter, r *http.Request)`](page.go) — Page writes the merged router settings and field descriptions for the dashboard to render.
- [`func PageMap(s Host) map[string]any`](page.go) — PageMap returns the merged router-settings document, including routing_groups.
- [`func Fields(s Host, rs map[string]any) []map[string]any`](page.go) — Fields splits router settings into the dashboard field list, with the current value and the default.
- [`func Callbacks(s Host, w http.ResponseWriter, r *http.Request)`](page.go) — Callbacks returns the callback view. Its router_settings match the merged document.
- [`func List(s Host, w http.ResponseWriter, r *http.Request)`](page.go) — List returns settings for the requested config_type. general_settings includes stored_in_db.

### route_settings.go

本文件导出 RouteSettings、ScopeRef、ScopeLookup 以及 Resolve、RequestChain、BuiltinSettings、BuiltinDocument、ValidateRouteTemplateDocument、TemplateRouting、ValidateTemplateCatalog 的别名。实现和方法契约见 [router/settings.go](../../router/settings.go) 与 [router/compile.go](../../router/compile.go)。本目录测试验证调用契约，解析和执行由 router 负责。

### settings.go

- [`func Base(s Host) map[string]any`](settings.go) — 返回四个全局执行设置：num_retries、timeout、allowed_fails、cooldown_time。YAML 值覆盖代码默认值；默认失败阈值为 3，cooldown 为 0。
- [`func MergedRouter(s Host) map[string]any`](settings.go) — MergedRouter overlays database router settings on the baseline. Keys absent from the database keep the YAML value.
- [`func MergedGeneral(s Host) map[string]any`](settings.go) — MergedGeneral overlays database general settings on YAML. master_key, database_url, and redis_url are not exposed from the database.
- [`func ApplyTyped(s Host, m map[string]any)`](settings.go) — 把合并文档中的 num_retries 和 timeout 写入类型化运行配置；缺失字段保持原值。失败阈值与冷却时间由路由使用时从合并文档读取。
- [`func Update(s Host, w http.ResponseWriter, r *http.Request)`](settings.go) — Update accepts a partial update of router, general, or LiteLLM settings. It requires a management identity.
- [`func GeneralList(s Host) []map[string]any`](settings.go) — GeneralList returns the general-settings list with stored_in_db. A key only in the database is true, a key only in YAML is false, and a key in neither is null.
- [`func FieldUpdate(s Host, w http.ResponseWriter, r *http.Request)`](settings.go) — FieldUpdate updates one general-settings field. A missing field_name returns 400.
- [`func FieldDelete(s Host, w http.ResponseWriter, r *http.Request)`](settings.go) — FieldDelete deletes one general-settings field. After deletion the YAML baseline applies again.

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
| --- | --- |
| `GET /router/settings` | [mount.go](mount.go) |
| `GET /router/fields` | [mount.go](mount.go) |
| `GET /get/config/callbacks` | [mount.go](mount.go) |
| `GET /config/list` | [mount.go](mount.go) |
| `POST /config/update` | [mount.go](mount.go) |
| `POST /config/field/update` | [mount.go](mount.go) |
| `POST /config/field/delete` | [mount.go](mount.go) |

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/config](../../config/readme_cn.md), [internal/httpx](../../httpx/readme_cn.md), [internal/iam](../../iam/readme_cn.md), [internal/logx](../../logx/readme_cn.md), [internal/store](../../store/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [route_settings_test.go](route_settings_test.go) | `TestNoTemplateUsesBuiltinModelDefault`, `TestNarrowestTemplateSelectionWins`, `TestUnselectedNarrowScopesInheritWiderTemplate`, `TestDanglingSelectionFallsBackToModelDefault`, `TestRequestChainIsNarrowestFirst`, `TestTypedExecutionDefaultsAndBoundaries`, `TestResolveWithoutLookupUsesModelDefault` |
| [allocation_test.go](allocation_test.go) | `TestTemplateOnlyOverridesNamedModels`, `TestBuiltinAndEmptyWeightsUseEqualTrafficSplit`, `TestStrictRouteTemplateDocument`, `TestTemplateDefaultStrategyAndOverride` |
| [routing_groups_test.go](routing_groups_test.go) | `TestRoutingGroupPrecedence` |
| [fallback_test.go](fallback_test.go) | `TestTemplateFallbackParsing`, `TestTemplateFallbackCatalog` |

```bash
go test ./internal/gateway/prefs -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
