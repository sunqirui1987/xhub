# 供应商能力与传输登记

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

capability.go 把入口能力与发送方式拆开；registry.go 注册 Transport、供应商默认地址和模型贡献；type.go 定义 Capability、Transport 及路径动作结构。RegisterTransport 校验 ID、动作和大写 HTTP 方法。
SelectedCapabilities 只读取 endpoint_types；缺失或空列表表示没有已声明能力，显式未知能力不进入适配池。SelectedTransport 只读取 transport；已登记的 bypass 才能选择官方转发。Ops 用于实际入口匹配，Paths 用于展示。
配置 api_base 覆盖供应商默认。OfficialID 只去掉一层供应商前缀并保留模型名后续斜杠；ReadTaskID 支持点路径，Expand 替换路径参数。登记新供应商必须检验能力、协议、路径、usage 和任务结算，RegisterModel 只贡献目录，不证明真实调用可用。

## 子目录与协作边界

| 目录 | 职责 |
| --- | --- |
| [all](all/readme_cn.md) | 内置供应商装配 |
| [openai](openai/readme_cn.md) | OpenAI 供应商占位边界 |
| [volcengine](volcengine/readme_cn.md) | 火山方舟官方视频任务传输 |

## 源码职责与入口

### capability.go

- [`func Capabilities() []Capability`](capability.go) — Capabilities 返回全部入口能力。
- [`func CapabilityForOp(op string) (string, bool)`](capability.go) — CapabilityForOp 返回应答这个数据面 op 的能力 id。认不出 op 时返回假。 适配路径用它过滤部署：一条只标了 embedding 的部署不该应答 /v1/chat/completions。

### registry.go

公开类型：`Model`, `ProviderField`, `Supplier`.

### seedance_billing.go


- [`func RegisterTransport(t Transport)`](registry.go) — RegisterTransport 登记一个内置转发方式。供应商文件在 init 里调它。
- [`func RegisterModel(m Model)`](registry.go) — RegisterModel 登记一条可选模型，并为表单记录默认执行传输 ID。
- [`func RegisterSupplier(s Supplier)`](registry.go) — RegisterSupplier 记下默认 API 根；名字是新的时，连添加模型的凭据字段一起记下。
- [`func APIBase(slug, configured string) string`](registry.go) — APIBase 返回部署上的根地址；部署没填时用供应商登记的默认根。
- [`func Transports() []Transport`](registry.go) — Transports 返回已登记的内置转发方式。
- [`func ModelEndpoints() map[string]string`](registry.go) — ModelEndpoints 把内置模型 id 映射到表单要预选的默认 transport ID。
- [`func PublicBody() map[string]any`](registry.go) — PublicBody 是添加模型的载荷：公开能力、执行传输和模型默认值。endpoint_types 是公开协议能力，capabilities 是能力与路径说明，transports 是后端登记的执行传输，models 是模型到默认 transport 的映射。
- [`func Match(method, path string, models []config.ModelEntry) (Hit, bool)`](registry.go) — Match 找出这个方法和路径命中的 bypass 动作，只在已登记的转发方式里找。 它刻意不读部署上的自定义文档：bypass 是后台登记的形状，不是运维在界面上 随手填的一份路径表。一份填错的路径表发不出请求，也就拿不到上游的返回值， 预选、日志和用量都无从谈起。 models（[]config.ModelEntry）：候选部署列表，当前不参与匹配，保留给调用方复用签名。
- `SelectedCapabilities` 只读取 `model_info.endpoint_types`。未声明或空列表不声明能力；显式填入未知能力时，不匹配任何能力。
- `SelectedTransport` 只读取 `model_info.transport`。登记过的 bypass ID 选择对应传输；未声明时使用 adapted。
- [`func IsAdapted(m config.ModelEntry) bool`](registry.go) — IsAdapted 报告这条部署走协议适配。Bypass 部署不能从能力门进适配路径： 方舟内容生成的入口是 /api/v3/contents/generations/tasks，不是 /v1/videos， 把它放进适配池会让 /v1/videos 选中它然后打错地址。
- AdaptedPool（registry.go）：适配路径只保留显式声明了对应操作能力的部署；空或未知操作返回空池，禁止未声明入口借用默认对话路径。
- [`func IncludesCapability(m config.ModelEntry, capability string) bool`](registry.go) — IncludesCapability 报告这条部署是否应答这个能力。
- [`func Includes(m config.ModelEntry, typeID string) bool`](registry.go) — Includes 报告这条部署是否选中了这个转发方式 id。Bypass 选部署用它： 路径先命中转发方式，再按转发方式 id 挑部署，能力不参与。
- [`func BoundTransports(m config.ModelEntry) []Transport`](registry.go) — BoundTransports 把这条部署声明的转发方式解析成登记好的条目。

### type.go


公开类型：`Action`, `Capability`, `Transport`, `Kind`, `Hit`.

- [`func ReadTaskID(doc map[string]any, path string) string`](type.go) — ReadTaskID 从 JSON 对象里读一个点分字段，例如 "data.task_id"。
- [`func Expand(pattern string, names map[string]string) string`](type.go) — Expand 把上游路径里的 {name} 占位符填上。

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/catalog](../catalog/readme_cn.md), [internal/config](../config/readme_cn.md), [internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [capability_test.go](capability_test.go) | `TestEveryDeclaredPathBelongsToItsOwnCapability`, `TestChatCoversThreeSpellings`, `TestCompletionIsNotPartOfChat`, `TestUnregisteredOpsAreNotCapabilities`, `TestImageCoversGenerationAndEdit`, `TestAdaptedPoolKeepsOnlyDeploymentsThatAnswerTheOp` |
| [match_test.go](match_test.go) | `TestBypassPathsStayOnTheirProviders`, `TestRegisteredTransportsAreTheOnlyBypassSource`, `TestReadTaskID` |
| [registry_test.go](registry_test.go) | `TestSelectedCapabilitiesUsesDeclaredIDs`, `TestTransportRequiresExplicitRegisteredID`, `TestOneModelCanAnswerSeveralCapabilities`, `TestBypassTypesStayWithTheirProvider`, `TestABypassIsNeverTakenFromADeploymentDocument` |

```bash
go test ./internal/provider -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
