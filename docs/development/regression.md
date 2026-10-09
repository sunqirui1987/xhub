# 全链路回归测试方案

[开发导航](README.md) · [功能实现](implementation.md) · [浏览器回归](e2e-regression.md) · [测试基础](testing.md)

同一公开模型由多个供应商或多个部署共同提供时，fixture、权重周期、重试/粘性边界和真实供应商证据要求见[多供应商同模型回归](multi-supplier-regression.md)。

## 验收边界与证据

后端回归入口是 [internal/regression](../../internal/regression/readme_cn.md)。测试启动真实网关，通过公开 HTTP 接口建用户、组织、团队、项目、密钥、模型与模板；PostgreSQL 使用各用例独立 schema。默认上游是可编排的本地服务器，以便确定性注入 400、429、500、超时、流事件和 usage。真实模型模式在相同网关和数据库链路下改为外部供应商；不能用 fake 的成功证明供应商协议仍可用，也不能用真实供应商的不稳定响应验证精确故障次数。

每个业务断言至少明确调用身份、前置资源、请求、成功或拒绝、上游是否被拨号、调用 ID、费用响应头、持久化日志以及 user/key/project/team/org 五层累计的预期变化。需要区分 HTTP 已返回与 Redis 队列已入库；读取数据库前由 harness 刷写。遇到 skip，要写明条件，不把跳过计为通过。

本套件覆盖当前定义的业务场景，不宣称穷尽所有输入、并发交错、供应商协议版本和未来新增功能。目录内的 [readme_cn.md](../../internal/regression/readme_cn.md) 列出所有顶层 Test 名称；本页解释每个文件所验证的行为。

## 文件与行为矩阵

| 文件 | 输入和流程 | 必须观察的结果 |
| --- | --- | --- |
| [doc.go](../../internal/regression/doc.go) | 包说明 | 不含测试入口，规定回归包职责。 |
| [harness_test.go](../../internal/regression/harness_test.go) | 启动网关、HTTP 测试客户端、独立数据库 schema、fake 上游和 live 配置 | 测试隔离，真实网络入口，失败响应可诊断；严格模式不静默跳过数据库。 |
| [mode_test.go](../../internal/regression/mode_test.go) | simulated/live 双模式调度、真实供应商构建 | 可注入故障的场景只在 simulated 验精确顺序；live 缺配置时明确 skip。 |
| [chain_support_test.go](../../internal/regression/chain_support_test.go) | 构建五层租户，封装模型调用和账务读取 | 单次成功费用等于响应、日志及五层累计增量。 |
| [setup_test.go](../../internal/regression/setup_test.go) | 供应商部署、作用域 key、模型名单 | 配置后可调用、名单限制生效。 |
| [chain_test.go](../../internal/regression/chain_test.go) | 完整请求链 | 选路、项目限制、回退和费用按同一租户链执行。 |
| [budget_test.go](../../internal/regression/budget_test.go) | 多次消费、速率限制、无限额 | 预算累加；RPM/TPM 与预算错误不同；无限额不误拒绝。 |
| [budget_chain_test.go](../../internal/regression/budget_chain_test.go) | user/team/project/org/key 各层预算及状态变更 | 最窄耗尽层被点名，拒绝不拨上游、不增加任何一层费用；提高上限后恢复。 |
| [model_limit_test.go](../../internal/regression/model_limit_test.go) | 项目/key 模型限制与变更 | 两层名单独立收窄，拒绝修改不影响现有可调用性。 |
| [permission_test.go](../../internal/regression/permission_test.go) | 双组织、成员与管理员、key 生命周期 | 跨租户读写拒绝；成员不可管理；轮换后旧 key 失效；匿名和错误 key 拒绝。 |
| [interaction_test.go](../../internal/regression/interaction_test.go) | 护栏/缓存/预算组合、管理审计、即时禁用 | 免费拦截不消费预算，配置更新即生效，删除团队使其 key 失效。 |
| [consistency_test.go](../../internal/regression/consistency_test.go) | 响应、日志详情、累计费用互查 | usage、金额、call ID 一致；缓存命中和失败有日志而无新增费用。 |
| [logs_test.go](../../internal/regression/logs_test.go) | 各日志视图、活动、正文存储开关、花费计算 | 租户隔离、同一 call ID、日统计与价格目录一致，正文遵循开关。 |
| [endpoints_test.go](../../internal/regression/endpoints_test.go) | chat、embedding、Seedance 任务创建与轮询 | 请求走相应上游路径；任务 ID 可回查；控制台无法随意伪造 bypass。 |
| [protocol_test.go](../../internal/regression/protocol_test.go) | 三种 chat 协议、流式、embedding、未知路径 | 响应与 usage 可解析，流式结算一致，不存在路径为 404。 |
| [models_test.go](../../internal/regression/models_test.go) | 发现、catalog、transport、部署增删/禁用、错误凭据 | 能力与可见性一致；无价不伪装有价；禁用或删除后不能调用。 |
| [routing_test.go](../../internal/regression/routing_test.go) | 排序策略、最少繁忙、精确名、会话粘性 | 候选顺序和实际拨号一致，未知策略走明确边界。 |
| [router_settings_test.go](../../internal/regression/router_settings_test.go) | 平台路由设置热更新 | 下一请求读新策略；仅保存的回退字段不被误认执行。 |
| [fallback_test.go](../../internal/regression/fallback_test.go) | 多部署失败、冷却和禁用 | 失败按规则换部署，冷却跳过且只为成功调用计一次。 |
| [fal_billing_test.go](../../internal/regression/fal_billing_test.go) | 模拟 fal 视频任务的创建、排队、处理中、失败和重复完成轮询 | 未完成或失败不收费；完成后以输出视频秒数计价，响应费用、五层累计、持久化账单明细一致且只结算一次。 |
| [split_test.go](../../internal/regression/split_test.go) | weighted-split 与普通排序 | 流量进入正权重部署，普通策略不被 split 权重改写，每次调用正常记账。 |
| [route_template_test.go](../../internal/regression/route_template_test.go) | 模板建立、组织/团队/key 选择、控制台 effective、引用删除 | 无选择用平台；低层覆盖高层；仍被引用拒删；创建时选择保留。 |
| [route_template_config_test.go](../../internal/regression/route_template_config_test.go) | 见下方完整模板矩阵 | 配置保存、解析来源与实际请求行为互相印证。 |
| [route_template_live_test.go](../../internal/regression/route_template_live_test.go) | 真实模型下绑定组织、团队、key，编辑 key 模板，再逐层清空 | 每次 effective 来源正确，模型确实回答，日志和五层累计都随每次调用增长。 |
| [weighted_live_test.go](../../internal/regression/weighted_live_test.go) | 从 `config_provider.yaml` 读取同模型多部署及权重，逐次调用真实模型 | 成功部署由会话钉住信息确认，完整周期内实际次数符合配置权重，并留存逐次证据。 |
| [weighted_live_metadata_test.go](../../internal/regression/weighted_live_metadata_test.go) | 在加载凭据或建立可能产生费用的 live harness 前解析并校验 provider metadata | 拒绝畸形 JSON、未知字段、无效版本或配置、全零权重和不完整周期；超大权重仍能无整数溢出地计算精确预期次数。 |
| [multi_supplier_test.go](../../internal/regression/multi_supplier_test.go) | 从 `internal/regression/testdata/config_provider.yaml` 加载共享 provider schema，并通过 `/model/new` 创建物理连接完全相同的数据库重复部署；覆盖权重、模板、作用域、重试、流式、并发、亲和、禁用与冷却 | 配置部署以 `litellm_params.deployment_id`、数据库部署以 `model_info.id` 区分；核对最终完整周期聚合、响应亲和、上游尝试、价格日志和五层账务；Redis 用例验证 A 冷却时 B 不受影响。 |
| [seedance_billing_test.go](../../internal/regression/seedance_billing_test.go) | 创建任务，轮询多种未成功状态，再重复轮询带真实计量形状的成功响应 | 只在成功且有完成 token 时结算；费用、快照变体、五层增量及持久化去重一致。 |
| [guardrail_test.go](../../internal/regression/guardrail_test.go) | 拦截、改写、模型范围与试运行 | 拦截前不拨号、不收费；改写后的内容送上游；试运行不落配置。 |
| [pricing_test.go](../../internal/regression/pricing_test.go) | 时间窗口、按秒、缓存和历史改价 | 窗口与数量×费率一致；未定价不当免费；旧快照不随新价改写。 |
| [pricing_units_test.go](../../internal/regression/pricing_units_test.go) | 控制台价格维度与 catalog 模型 | token/图片/秒/请求、peak 与缓存的单位和部署名可核对。 |
| [pricing_live_test.go](../../internal/regression/pricing_live_test.go) | 真实 provider usage、流事件、双协议与价格目录 | 真实 quantity、适用费率、金额与快照独立比较；无可比 token 费率不能误判相等。 |
| [live_test.go](../../internal/regression/live_test.go) | 显式配置的真实模型与媒体任务 | 模型返回正 usage/正价格；配置了 bypass 才创建和轮询真实任务。 |
| [more_cases_test.go](../../internal/regression/more_cases_test.go) | 幂等重放、坏请求、TPM、429、家族路径 | 重放不重收；流式不误复用；非法请求不拨号；各协议进入自身路径。 |
| [reset_test.go](../../internal/regression/reset_test.go) | 花费重置和密码重置 | 额度变化与账号会话失效符合持久化结果。 |
| [removed_surfaces_test.go](../../internal/regression/removed_surfaces_test.go) | 已退休接口和正常调用 | 退休路径仍不可用，保留功能不受影响。 |

## 路由模板配置专项

[route_template_config_test.go](../../internal/regression/route_template_config_test.go) 的 12 条用例把表单字段、管理接口和数据面连起来：

1. **RoundTrip**：对照 `router/settings` 字段清单，创建、列表、详情、整体替换；检查 0、false、null、空数组与高级字段原样保留。高级回退、别名和 stream_timeout 等仅证明持久化。
2. **SeedsOnce**：创建时复制当前平台值，平台之后修改不污染已建模板；清除绑定才继承新平台。
3. **Precedence**：按 key→team→organization→platform 选择整份文档；逐层清空，同时核对 effective 和真实拨号顺序/重试次数。
4. **Edits**：更新模板后下一请求读新值，未绑定的租户仍用原策略。
5. **RetriesAndFailover**：500/429 在单部署耗尽重试后换部署；普通 400 终止且不收费，成功只计一笔；跨公开模型的 stored fallbacks 不会被当作已执行。
6. **Weights**：列表和 map 两种权重配置，10 次调用精确 3:7 分配并核对五层计费。
7. **ModelRouting**：两个公开模型分别应用专属加权分流和成本策略，未列模型沿用顶层策略；同一模板的重试和未知字段保留。
8. **Timeout**：慢上游在短时限失败，改为较长时限后无需重启即可成功。
9. **Cooldown**：真实 Redis 下验证模板阈值和 TTL 相对于平台默认的选择；清空模板不会擦掉已存在的共享冷却。
10. **ScopeForms**：常规组织、团队、密钥表单保存保留/清空选择，不仅测试专用 binding API。
11. **UsageAndDeletion**：各作用域的引用可见，引用期间拒删。
12. **Permissions**：跨组织绑定和修改被拒绝，不发生部分写入。

真实供应商专项 [route_template_live_test.go](../../internal/regression/route_template_live_test.go) 核对选择、编辑、继承、实际推理、日志与五层费用；[weighted_live_test.go](../../internal/regression/weighted_live_test.go) 对配置的真实同模型部署逐次核对分流周期。精确超时、错误码与 Redis 冷却仍需确定性上游，因为外部供应商不能可靠地产生指定故障。

## 执行流程

先运行 Go 单元测试及静态检查；准备 PostgreSQL（默认本地 5433）和可选 Redis；运行严格确定性回归；再显式打开真实供应商模式；最后执行浏览器回归，检查报告与 skip。推荐命令：

```bash
go test ./internal/... -count=1
go vet ./internal/...
make regression
E2E_CREDENTIAL_SOURCE=database make regression-live
E2E_CREDENTIAL_SOURCE=database make e2e
```

真实供应商的地址、模型与协议默认来自根目录 `config_provider.yaml`，密钥来自配置的 `key_env`，或显式选择的数据库凭据。环境变量 `_BASE`、`_MODELS`、`_PROTOCOL` 可覆盖普通调用配置；媒体另需 `_BYPASS_MODEL`、`_BYPASS_BASE`。后端脚本默认总超时 1800s，可用 `XHUB_REGRESSION_TIMEOUT` 调整。数据库可用 `XHUB_TEST_DATABASE_URL` 指定；Redis 可用 `XHUB_REGRESSION_REDIS_URL` 指定，完整 E2E 入口自动创建隔离 Redis。每个同时运行的回归或 E2E runner 必须使用独立 Redis 实例，不能让多个进程共享同一花费队列和冷却 key。密钥不要写入文档或日志。只运行指定测试时使用 `go test ./internal/regression -run '^TestName$' -count=1 -v`，避免把 skip 混入通过结果。

`config_provider.yaml` 可以定义供应商和真实权重场景；`scripts/with-live-vendors.py` 读取它，按 `key_env` 从进程环境或显式选择的数据库凭据加载密钥，然后只在子进程注入测试变量。完整浏览器与后端验收运行 `bash scripts/e2e-all.sh`，结果及逐次权重证据位于 `.e2e/runs/`。真实供应商网络调用可能很慢；若中断，报告必须标明尚未完成，不能把已成功的单次响应算作整套通过。

严格模式解决数据库不可达时测试全部跳过却显示绿色的问题。Redis 缺失时冷却专项会跳过；共享 Redis 被同时运行的其他测试进程使用时，花费日志和冷却状态会相互污染，不能作为有效验收。未提供某供应商或媒体凭据时对应 live 场景跳过；未配置同一供应商双协议时双协议比价跳过。真实供应商可能因限流或供应商故障失败，需保留失败证据并与产品回归失败区分。

验收时记录命令、代码版本、测试数、pass/fail/skip、Redis 与数据库是否到位、真实供应商与模型标识、usage/费用是否核对、浏览器报告位置。不要在正式文档中固化某次运行日志或敏感连接信息；当行为变化时更新上述矩阵和对应目录 README。
