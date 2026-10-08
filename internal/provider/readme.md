# Provider capabilities and transports

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

Capabilities describe accepted operations; transports describe sending behavior. Registration validates identifiers, actions, and uppercase methods. Supplier defaults and model contributions are separate catalog inputs.
SelectedCapabilities prefers endpoint_types, then recognized legacy mode, then implicit chat. Explicit unknown capabilities do not silently become chat. Ops drive matching; Paths are presentation. Official bypass matches registered public paths only; deployment-local descriptions cannot create new gateway routes.
Configured api_base overrides supplier defaults. OfficialID strips one prefix while preserving remaining slashes; ReadTaskID supports dotted extraction and Expand fills placeholders. ApplyOverride is currently a compatibility no-op. Catalog contribution is not proof of working live inference.

## Subdirectories and collaboration

| Directory | Responsibility |
| --- | --- |
| [all](all/readme.md) | Built-in provider assembly |
| [openai](openai/readme.md) | OpenAI provider placeholder |
| [qiniu](qiniu/readme.md) | Qiniu official video-task transport |
| [volcengine](volcengine/readme.md) | Volcengine Ark video-task transport |

## Source responsibilities and entry points

### capability.go

- [`func Capabilities() []Capability`](capability.go) — Capabilities 返回全部入口能力。
- [`func CapabilityForOp(op string) (string, bool)`](capability.go) — CapabilityForOp 返回应答这个数据面 op 的能力 id。认不出 op 时返回假。 适配路径用它过滤部署：一条只标了 embedding 的部署不该应答 /v1/chat/completions。

### registry.go

Exported types: `Model`, `ProviderField`, `Supplier`.

- [`func RegisterTransport(t Transport)`](registry.go) — RegisterTransport 登记一个内置转发方式。供应商文件在 init 里调它。
- [`func RegisterModel(m Model)`](registry.go) — RegisterModel 登记一条可选模型。选中这个模型 id 时，表单会带上它的端点类型。
- [`func RegisterSupplier(s Supplier)`](registry.go) — RegisterSupplier 记下默认 API 根；名字是新的时，连添加模型的凭据字段一起记下。
- [`func APIBase(slug, configured string) string`](registry.go) — APIBase 返回部署上的根地址；部署没填时用供应商登记的默认根。
- [`func Transports() []Transport`](registry.go) — Transports 返回已登记的内置转发方式。
- [`func ModelEndpoints() map[string]string`](registry.go) — ModelEndpoints 把价目表模型 id 映射到表单要预选的端点类型。
- [`func PublicBody() map[string]any`](registry.go) — PublicBody 是添加模型的载荷：能力表、转发方式表和模型默认值。 形状从 {types, models} 改成了 {capabilities, transports, models}， 因为原来的 types 正是这次要拆掉的那件东西。调用方只有添加模型表单。
- [`func Match(method, path string, models []config.ModelEntry) (Hit, bool)`](registry.go) — Match 找出这个方法和路径命中的 bypass 动作，只在已登记的转发方式里找。 它刻意不读部署上的自定义文档：bypass 是后台登记的形状，不是运维在界面上 随手填的一份路径表。一份填错的路径表发不出请求，也就拿不到上游的返回值， 预选、日志和用量都无从谈起。 models（[]config.ModelEntry）：候选部署列表，当前不参与匹配，保留给调用方复用签名。
- [`func ApplyOverride(m config.ModelEntry, hit Hit) Hit`](registry.go) — ApplyOverride 当前是恒等函数。部署上不再支持自带 bypass 文档， 保留这个签名让 dataplane 的调用点不必改。
- [`func SelectedCapabilities(m config.ModelEntry) []string`](registry.go) — SelectedCapabilities 返回一条部署应答的能力 id 列表。 读顺序： 1. model_info.endpoint_types：新写入是这个字段，里面是能力 id。 2. model_info.mode：旧行只有这个字符串，按存量 id 映射成能力。 3. 都没有：chat。老的部署和不带端点信息的部署都是这个意思。 认不出的 id（realtime、batch、ocr）被忽略，不放进任何能力。一项都认不出时 realtime 的部署意外应答所有对话请求。
- [`func SelectedTransport(m config.ModelEntry) string`](registry.go) — SelectedTransport 返回一条部署的转发方式 id。 判定顺序： 1. model_info.transport 是登记过的内置 id → 那个 id。 2. endpoint_types 或 mode 里出现内置 Bypass id → 那个 id。旧行只写了这个。 3. 其余 → adapted。 内置 Bypass 之外的 bypass 形状不存在：后台没登记过的转发方式，运维在界面上 也选不到、存不进。
- [`func IsAdapted(m config.ModelEntry) bool`](registry.go) — IsAdapted 报告这条部署走协议适配。Bypass 部署不能从能力门进适配路径： 方舟内容生成的入口是 /api/v3/contents/generations/tasks，不是 /v1/videos， 把它放进适配池会让 /v1/videos 选中它然后打错地址。
- [`func AdaptedPool(models []config.ModelEntry, op string) []config.ModelEntry`](registry.go) — AdaptedPool 把适配路径的候选收敛到能应答这个 op 的部署。 两件事都做：丢掉不是协议适配的，丢掉能力不含这个 op 的。这是新行为， 不是把现有比较换个写法——原来适配路径完全不过滤端点类型，一条标成 embedding 的部署现在仍能被 /v1/chat/completions 打到。
- [`func IncludesCapability(m config.ModelEntry, capability string) bool`](registry.go) — IncludesCapability 报告这条部署是否应答这个能力。
- [`func Includes(m config.ModelEntry, typeID string) bool`](registry.go) — Includes 报告这条部署是否选中了这个转发方式 id。Bypass 选部署用它： 路径先命中转发方式，再按转发方式 id 挑部署，能力不参与。
- [`func SelectedTypes(m config.ModelEntry) []string`](registry.go) — SelectedTypes 返回一条部署声明的原始端点 id 列表。它只服务 Bypass 选部署： 能力那一路走 SelectedCapabilities。endpoint_types 优先，其次 mode，都没有则 chat。
- [`func BoundTransports(m config.ModelEntry) []Transport`](registry.go) — BoundTransports 把这条部署声明的转发方式解析成登记好的条目。

### type.go

Exported types: `Action`, `Capability`, `Transport`, `Kind`, `Hit`.

- [`func OfficialID(prefix, stored string) string`](type.go) — OfficialID 从存储的模型 id 上剥掉一层 "<前缀>/"。后面的斜杠保留， 所以 qiniu/bytedance/doubao-... 会变成 bytedance/doubao-...
- [`func ReadTaskID(doc map[string]any, path string) string`](type.go) — ReadTaskID 从 JSON 对象里读一个点分字段，例如 "data.task_id"。
- [`func Expand(pattern string, names map[string]string) string`](type.go) — Expand 把上游路径里的 {name} 占位符填上。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/catalog](../catalog/readme.md), [internal/config](../config/readme.md), [internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [capability_test.go](capability_test.go) | `TestEveryDeclaredPathBelongsToItsOwnCapability`, `TestChatCoversThreeSpellings`, `TestCompletionIsNotPartOfChat`, `TestUnregisteredOpsAreNotCapabilities`, `TestImageCoversGenerationAndEdit`, `TestAdaptedPoolKeepsOnlyDeploymentsThatAnswerTheOp` |
| [match_test.go](match_test.go) | `TestBypassPathsStayOnTheirProviders`, `TestRegisteredTransportsAreTheOnlyBypassSource`, `TestReadTaskID` |
| [registry_test.go](registry_test.go) | `TestSelectedCapabilitiesReadsBothSpellings`, `TestOneModelCanAnswerSeveralCapabilities`, `TestBypassTypesStayWithTheirProvider`, `TestABypassIsNeverTakenFromADeploymentDocument` |

```bash
go test ./internal/provider -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
