# API 家族处理与资源隔离

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

mount.go 显式挂载 Responses 的两个入口，handlers.go 根据路径识别 inferenceOp，已登记的其它协议入口由网关目录调度。codec.go 处理请求与错误封装，host.go 定义调用数据面和本地存储所需能力。
支持的推理动作会交给统一数据面，不能在协议入口绕过预算、模型权限和模板。Assistants/Threads 的本地元数据按租户隔离，知道对象 ID 不能读取另一调用方的对象。
异步任务、files、batch、fine-tune、realtime 等未实现的 API 返回 501 或对应明确失败，而不是模拟成功。Responses 的 previous_response_id 是粘性输入；完整协议兼容性需要固定样本和真实供应商测试，不以家族路径登记替代验证。

## 源码职责与入口

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### handlers.go

- [`func ServeDataPlane(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — ServeDataPlane handles inference paths in the catalog. A recognized operation enters the data plane. The rest are resource reads and writes.
- [`func Responses(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — Responses is the Responses API entry and always uses the data-plane responses operation.
- [`func ServeMixed(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — ServeMixed answers the catalog's "mixed" paths, such as /v1/agents, /v1/skills and /v1/workflows. These were removed from the product and only remain in the catalog. They are refused rather than served: the generic store behind them is one shared key-value namespace keyed by resource kind, with no owner and no team column, so serving them let any signed-in member — and any inference key — create records that every other principal could then list. Refusing is what the classification has always described; this makes the code match. The identity is still required before the refusal, so an anonymous caller learns nothing beyond the fact that the path exists.
- [`func ServeMgmt(s Host, w http.ResponseWriter, r *http.Request)`](handlers.go) — ServeMgmt handles management catalog paths. It requires a management identity, then reads or writes by resource kind.
- [`func Freeze(kind string, obj map[string]any)`](handlers.go) — Freeze fills fields the public contract for this resource family requires. An existing value is not overwritten.

### host.go

公开类型：`Host`.

内部实现和协议边界见 [host.go](host.go)。

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module sends the Responses API to the data plane. Other catalog resources are not part of this module.

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
| --- | --- |
| `POST /v1/responses` | [mount.go](mount.go) |
| `POST /responses` | [mount.go](mount.go) |

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/catalog](../../catalog/readme_cn.md), [internal/dataplane](../../dataplane/readme_cn.md), [internal/httpx](../../httpx/readme_cn.md), [internal/logx](../../logx/readme_cn.md), [internal/store](../../store/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [resource_isolation_test.go](resource_isolation_test.go) | `TestInferenceResourcesAreIsolatedByCaller`, `TestUnsupportedInferenceDoesNotFabricateUsage`, `TestInjectModelPreservesInvalidPayloadForValidation` |

```bash
go test ./internal/gateway/family -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
