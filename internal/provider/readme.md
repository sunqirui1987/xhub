# Provider capabilities and transports

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

Capabilities describe accepted operations; transports describe sending behavior. Registration validates identifiers, actions, and uppercase methods. Supplier defaults and model contributions are separate catalog inputs.
SelectedCapabilities reads endpoint_types only. Missing or empty declarations mean no declared capability; explicitly unknown capabilities match no adapted operation. SelectedTransport reads transport only and selects registered bypass transports. Ops drive matching; Paths are presentation.
Configured api_base overrides supplier defaults. OfficialID strips one prefix while preserving remaining slashes; ReadTaskID supports dotted extraction and Expand fills placeholders. Catalog contribution is not proof of working live inference.

## Subdirectories and collaboration

| Directory | Responsibility |
| --- | --- |
| [all](all/readme.md) | Built-in provider assembly |
| [openai](openai/readme.md) | OpenAI provider placeholder |
| [volcengine](volcengine/readme.md) | Volcengine Ark video-task transport |

## Source responsibilities and entry points

### candidates.go

- [`func Candidates(list []config.ModelEntry, endpoint string, dialogue *llm.Dialogue) ([]config.ModelEntry, []CandidateDecision)`](candidates.go) — Candidates keeps deployments that explicitly allow the requested endpoint and, for dialogue requests, the declared protocol; it also returns a decision for every input deployment.

### execution.go

- [`func Execution(m config.ModelEntry) (Transport, bool)`](execution.go) — Execution resolves the deployment's registered transport; declarations that are absent or unknown do not invent a transport.
- [`func DialogueProtocol(protocol string) bool`](execution.go) — DialogueProtocol reports whether a protocol uses the shared dialogue conversion path.
- [`func AllowsEndpoint(m config.ModelEntry, endpoint string) error`](execution.go) — AllowsEndpoint validates that a deployment explicitly declares the requested endpoint.
- [`func ResolveHit(hit Hit, m config.ModelEntry) (Hit, error)`](execution.go) — ResolveHit applies a deployment's selected transport to a matched bypass action.

### capability.go

- [`func Capabilities() []Capability`](capability.go) — Capabilities 返回全部入口能力。
- [`func CapabilityForOp(op string) (string, bool)`](capability.go) — CapabilityForOp 返回应答这个数据面 op 的能力 id。认不出 op 时返回假。 适配路径用它过滤部署：一条只标了 embedding 的部署不该应答 /v1/chat/completions。

### registry.go

Exported types: `Model`, `ProviderField`, `Supplier`.

### seedance_billing.go


- [`func RegisterTransport(t Transport)`](registry.go) — RegisterTransport 登记一个内置转发方式。供应商文件在 init 里调它。
- [`func RegisterModel(m Model)`](registry.go) — RegisterModel 登记一条可选模型，并为表单记录默认执行传输 ID。
- [`func RegisterSupplier(s Supplier)`](registry.go) — RegisterSupplier 记下默认 API 根；名字是新的时，连添加模型的凭据字段一起记下。
- [`func Transports() []Transport`](registry.go) — Transports 返回已登记的内置转发方式。
- [`func ModelEndpoints() map[string]string`](registry.go) — ModelEndpoints 把内置模型 id 映射到表单要预选的默认 transport ID。
- [`func PublicBody() map[string]any`](registry.go) — PublicBody 是添加模型的载荷：公开能力、执行传输和模型默认值。endpoint_types 是公开协议能力，capabilities 是能力与路径说明，transports 是后端登记的执行传输，models 是模型到默认 transport 的映射。
- [`func Match(method, path string, models []config.ModelEntry) (Hit, bool)`](registry.go) — Match 找出这个方法和路径命中的 bypass 动作，只在已登记的转发方式里找。 它刻意不读部署上的自定义文档：bypass 是后台登记的形状，不是运维在界面上 随手填的一份路径表。一份填错的路径表发不出请求，也就拿不到上游的返回值， 预选、日志和用量都无从谈起。 models（[]config.ModelEntry）：候选部署列表，当前不参与匹配，保留给调用方复用签名。
- SelectedCapabilities (registry.go): reads only model_info.endpoint_types. Missing or empty declarations declare no capability; explicitly unknown nonempty declarations match no capability.
- SelectedTransport (registry.go): reads only model_info.transport. A registered bypass ID selects that transport; otherwise the deployment uses adapted transport.
- [`func IncludesCapability(m config.ModelEntry, capability string) bool`](registry.go) — IncludesCapability 报告这条部署是否应答这个能力。
- [`func Includes(m config.ModelEntry, typeID string) bool`](registry.go) — Includes 报告这条部署是否选中了这个转发方式 id。Bypass 选部署用它： 路径先命中转发方式，再按转发方式 id 挑部署，能力不参与。
- [`func BoundTransports(m config.ModelEntry) []Transport`](registry.go) — BoundTransports 把这条部署声明的转发方式解析成登记好的条目。

### type.go


Exported types: `Action`, `Capability`, `Transport`, `Kind`, `Hit`.

- [`func ReadTaskID(doc map[string]any, path string) string`](type.go) — ReadTaskID 从 JSON 对象里读一个点分字段，例如 "data.task_id"。
- [`func Expand(pattern string, names map[string]string) string`](type.go) — Expand 把上游路径里的 {name} 占位符填上。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/catalog](../catalog/readme.md), [internal/config](../config/readme.md), [internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [candidates_test.go](candidates_test.go) | `TestCandidatesUseDeclaredProtocols`, `TestDialogueBindingsUseEndpointDirectory` |
| [capability_test.go](capability_test.go) | `TestEveryDeclaredPathBelongsToItsOwnCapability`, `TestChatCoversThreeSpellings`, `TestCompletionIsNotPartOfChat`, `TestUnregisteredOpsAreNotCapabilities`, `TestImageCoversGenerationAndEdit` |
| [match_test.go](match_test.go) | `TestBypassPathsStayOnTheirProviders`, `TestRegisteredTransportsAreTheOnlyBypassSource`, `TestReadTaskID` |
| [registry_test.go](registry_test.go) | `TestSelectedCapabilitiesUsesDeclaredIDs`, `TestTransportRequiresExplicitRegisteredID`, `TestOneModelCanAnswerSeveralCapabilities`, `TestBypassTypesStayWithTheirProvider` |

```bash
go test ./internal/provider -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
