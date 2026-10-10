# 部署过滤、排序与加权分流

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md) · [路由契约](../../docs/development/routing.md)

## 职责与实现契约

router 按公开模型名或路由组匹配部署，过滤禁用和冷却候选，再生成尝试顺序；它不授予模型访问权限。`simple-shuffle` 与 `random` 均匀随机且忽略权重。`traffic-split` 每次按正权重的相对比例独立随机抽样，不是平滑加权轮询，有限样本不保证精确比例。

显式分配只使用 `deployment_id`。明细未列出的部署权重为 `1`；空 `allocations` 使所有匹配部署权重为 `1`。权重必须是有限非负数且至少有一个正值。`traffic-split` 排除零权重、冷却、禁用和非法权重部署；其他策略排除冷却部署，候选全不可用时返回空。会话固定部署只有在仍兼容、未冷却且满足权重条件时才能置顶。

`cost-based-routing` 比较当前时段的输入 token 单价，未定价候选排在最后。冷却和指标隔离身份优先使用 `pricing_id`，其次 `deployment_id`，最后 `model_info.id`。部署公开身份只使用 `litellm_params.deployment_id`，其次 `model_info.id`，不把 `pricing_id` 当作部署 ID。

## 源码职责与入口

- [`router.go`](router.go)：匹配候选、校验策略、过滤禁用部署并排序或选择。
- [`schedule.go`](schedule.go)：组合匹配、健康与权重过滤、会话粘性，生成请求尝试列表。
- [`policy.go`](policy.go)：解析和校验分配策略。
- [`settings.go`](settings.go)：读取模板契约及重试策略默认值。
- [`compile.go`](compile.go)：合并模型规则、平台默认值和回退设置。
- [`groups.go`](groups.go)：校验并展开路由组。
- [`fallback.go`](fallback.go)：校验回退图。
- [`template.go`](template.go)：校验模板引用及回退配置。
- [`weight_cleanup.go`](weight_cleanup.go)：模型目录变化时清理失效分配行。

本目录导出供 gateway 和 dataplane 调用的调度 API，但不直接注册 HTTP 路由。

## 模板契约

新模板使用 `model_routes` 和完整的 `retry_policy`。模型规则包含 `model`、`strategy` 及可选 `allocations`；只有 `traffic-split` 可配置分配，分配项为 `deployment_id` 与 `weight`。`max_attempts`、`timeout_seconds`、`failure_threshold`、`cooldown_seconds` 从 `retry_policy` 读取。路由组以及通用、上下文窗口、内容策略回退会经过校验并由请求链执行。

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [`allocation_test.go`](allocation_test.go) | `TestRandomAndTrafficIntervals`、`TestStickySchedulingDoesNotDraw`、`TestAllocationPolicyValidation`、`TestRelativeDefaultWeights`、`TestSimpleShuffleIsUniformRandom` |
| [`compile_test.go`](compile_test.go) | `TestCompileOverrides`、`TestCompileFailures`、`TestCompileGroupIsolation`、`TestCompileModelWeights` |
| [`compile_fallback_test.go`](compile_fallback_test.go) | `TestCompileFallbackPrecedence`、`TestCompileFallbackFailures` |
| [`groups_test.go`](groups_test.go) | `TestGroupContract`、`TestGroupScheduleIdentity` |
| [`fallback_test.go`](fallback_test.go) | `TestFallbackPolicy`、`TestFallbackGraph` |
| [`fallback_boundary_test.go`](fallback_boundary_test.go) | `TestFallbackPolicyCategoryBoundaries`、`TestFallbackGraphSharedAndDisconnected` |
| [`cost_regression_test.go`](cost_regression_test.go) | `TestCostRoutingUsesSettlementRatePrecedenceAndWindow`、`TestCostRoutingAcceptsValidNumericRates` |
| [`disabled_test.go`](disabled_test.go) | `TestAllExcludesDisabledExactAndWildcardDeployments` |
| [`weight_cleanup_test.go`](weight_cleanup_test.go) | `TestCleanAllocations`、`TestCleanTemplateWeights` |

实现变更时运行 router 包的定向测试；数据库、Redis 和真实供应商覆盖需要对应环境，不能由本包测试默认推断。

## 依赖关系

[internal/catalog](../catalog/readme_cn.md)、[internal/config](../config/readme_cn.md)、[internal/llm](../llm/readme_cn.md)、[internal/logx](../logx/readme_cn.md)。
