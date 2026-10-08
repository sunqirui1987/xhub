# 部署过滤、排序与加权分流

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

router.go 选择同公开模型名的部署并按策略排序，adapter.go 适配运行状态读取，split.go 实现平滑加权分流。禁用部署被过滤；冷却状态优先用扩展标识，兼容旧标识。
DeploymentID 是 api_base|model，用于模板权重；CooldownID 再加入 pricing/deployment ID 与命名凭据，不能放原始秘密。ApplyWeights 复制参数后覆盖份额，不能修改共享配置。simple-shuffle 当前偏向最大权重，weighted-split 才按比例累计调度；未提供权重等份，明确零排除。
router 只排序已授权候选，不授予模型访问。普通策略全冷却时仍尝试，split 没正权重开放候选则为空。成本策略比较当前输入 token 单价，不是全请求费用预测。粘性只能调整入选列表顺序，不能恢复被过滤部署。

## 源码职责与入口

### adapter.go

- [`func EncodeRequest(op, provider string, body map[string]any, realModel string) ([]byte, error)`](adapter.go) — EncodeRequest turns the public JSON body into the upstream request body. The protocol details live in internal/llm. This wrapper keeps the old name so callers do not each import that package.
- [`func DecodeResponse(op, provider, alias string, raw []byte) []byte`](adapter.go) — DecodeResponse turns an upstream response into the public shape and puts the caller's model alias back into the model field.

### router.go

公开类型：`State`.

- [`func All(list []config.ModelEntry, alias string) []config.ModelEntry`](router.go) — All returns every deployment under one public model name, before a strategy orders them.
- [`func Order(list []config.ModelEntry, alias, strategy string, st State) []config.ModelEntry`](router.go) — Order sorts usable deployments into attempt order for a strategy. A cooling deployment is not placed first when another deployment exists.
- [`func Pick(list []config.ModelEntry, alias, strategy string, st State) *config.ModelEntry`](router.go) — Pick returns the first deployment from Order. It returns nil when no deployment is usable.
- [`func DeploymentID(e config.ModelEntry) string`](router.go) — DeploymentID is the physical deployment identity, shaped as api_base|model. Weight overrides use this stable frontend-facing id.
- [`func CooldownID(e config.ModelEntry) string`](router.go) — CooldownID is the runtime deployment identity. It isolates cooldown, busy, latency, usage, session pinning, and billing state for named credentials that share one physical endpoint. A configured pricing_id or deployment_id is included when present so rows with the same endpoint, model, and credential name remain distinct. No API key is included.
- [`func IsSplitStrategy(strategy string) bool`](router.go) — IsSplitStrategy reports whether strategy divides traffic by weight. Hyphens and underscores are the same name. Anything else, including simple-shuffle, is not a split: simple-shuffle still picks the heaviest deployment, and treating it as a split would change that.
- [`func ApplyWeights(list []config.ModelEntry, overrides map[string]float64) []config.ModelEntry`](router.go) — ApplyWeights copies list and sets weight on the deployments named in overrides. The key is DeploymentID (api_base|model). A deployment that is not in the map keeps the weight already on it, which defaults to 1 inside the split. An empty map returns the same slice, so a document that does not configure shares does not allocate or change the pool. The copy matters: ModelList is the process config, and writing weight onto it would leak one request's template into the next request.
- [`func AdapterURL(provider, apiBase, realModel string) string`](router.go) — AdapterURL is the upstream address for chat completions. Other operations use AdapterURLOp.
- [`func AdapterURLOp(op, provider, apiBase, realModel string) string`](router.go) — AdapterURLOp returns the full URL for an operation and a provider. The rules live in internal/llm.Endpoint.
- [`func ValidateStrategy(strategy string) error`](router.go) — ValidateStrategy accepts the strategy names from the catalog and the hyphenated spellings the gateway config already uses. An unknown name returns an error and is not treated as simple-shuffle.

### split.go

公开类型：`SplitState`.

- [`func NewSplitState() *SplitState`](split.go) — NewSplitState returns an empty split state. One instance is shared by a process. The counters are per-process, so several replicas each converge on the configured ratio independently rather than coordinating one global schedule; the aggregate ratio is right either way.
- [`func SharedSplit() *SplitState`](split.go) — SharedSplit returns the process-wide split cursors. The router State is rebuilt for every request, so the cursors cannot live on it - a split that forgot where it was would send every request to the same deployment. One shared instance is what makes the ratio hold across requests.
- [`func (s *SplitState) PickWeighted(ids []string, weights []float64, available []bool) int`](split.go) — PickWeighted chooses the deployment that is furthest behind its share. This is smooth weighted round-robin: each turn every candidate's score grows by its weight, the highest score wins, and the winner's score drops by the total. Over any ten draws a 3:7 split lands exactly 3 and 7 rather than merely averaging that over a long run, which matters because a caller that watches ten consecutive requests should see the ratio they configured. A candidate that is not available has its score forgotten, so it re-enters at zero rather than immediately claiming the share it accrued while it was cooling down. That would otherwise send the first requests after a recovery all to the deployment that just came back. available（[]bool）：每条候选此刻是否可以接流量。
- [`func (s *SplitState) Forget(id string)`](split.go) — Forget drops a deployment's cursor. A deployment that is removed or renamed would otherwise leave its score behind forever, which is a slow leak in a process that runs for weeks.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/catalog](../catalog/readme_cn.md), [internal/config](../config/readme_cn.md), [internal/llm](../llm/readme_cn.md), [internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [cost_regression_test.go](cost_regression_test.go) | `TestCostRoutingUsesSettlementRatePrecedenceAndWindow`, `TestCostRoutingAcceptsValidNumericRates` |
| [disabled_test.go](disabled_test.go) | `TestAllExcludesDisabledExactAndWildcardDeployments` |
| [split_test.go](split_test.go) | `TestWeightedSplitFollowsTheConfiguredRatio`, `TestWeightedSplitDoesNotRequireHundred`, `TestSplitWithoutWeightsIsEven`, `TestSplitSkipsACoolingDeployment`, `TestSplitIsEvenAfterACoolingDeploymentReturns`, `TestSplitWithOneDeploymentDoesNotDisturbIt`, `TestSplitWithoutStateFallsBackToHighestWeight`, `TestApplyWeightsUsesTheDocumentWithoutTouchingThePool`, `TestWeightedSplitIsItsOwnStrategy`, `TestSplitKeepsRatioAcrossManyDraws`, `TestCostStrategyDoesNotLetAnUnpricedDeploymentWin` |
| [template_regression_test.go](template_regression_test.go) | `TestSplitExclusionsApplyToEveryAttempt`, `TestNamedCredentialCooldownAndRetryIsolation`, `TestCooldownIDUsesConfiguredStablePricingIdentity`, `TestRuntimeMetricsUseCredentialAwareIDs`, `TestRuntimeMetricsAcceptLegacyPhysicalID` |

```bash
go test ./internal/router -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
