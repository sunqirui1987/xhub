# 多供应商同模型回归

[开发导航](README.md) · [全链路回归](regression.md) · [浏览器回归](e2e-regression.md)

## 验收目标

本专项验证两个或更多供应商同时提供同一个公开模型名时，网关仍按部署身份完成加权选路、模板覆盖、失败转移、响应粘性、冷却隔离和结算。确定性用例建立两个可独立识别的本地供应商，既覆盖各自使用独立 HTTP 服务器，也覆盖共享同一 endpoint、使用不同合成凭据的情况。两者暴露相同的公开模型 `gpt-5.6-sol` 和上游模型名；另一个 `gpt-5.6-terra` 用来证明模型规则与作用域不会串扰。每个部署必须有稳定且唯一的 `deployment_id`，断言以部署为单位，不能从相同的模型名猜测供应商。

确定性测试只读取 `internal/regression/testdata/config_provider.yaml`，并通过 `internal/providerconfig.Load(io.Reader)` 解析版本 1 的共享 schema。它不读取仓库根目录的 `config_provider.yaml`：

```yaml
version: 1
providers:
  - id: SUPPLIER_A
    enabled: true
    credential_name: regression-supplier-a
    key_env: REGRESSION_FAKE_A_KEY
    base: http://supplier-a.invalid/v1
    protocol: openai
    models: [gpt-5.6-sol]
  - id: SUPPLIER_B
    enabled: true
    credential_name: regression-supplier-b
    key_env: REGRESSION_FAKE_B_KEY
    base: http://supplier-b.invalid/v1
    protocol: openai
    models: [gpt-5.6-sol]
weighted_scenarios:
  - name: same-model-across-suppliers
    model_name: gpt-5.6-sol
    requests: 20
    deployments:
      - id: sol-a
        provider: SUPPLIER_A
        model: gpt-5.6-sol
        weight: 7
      - id: sol-b
        provider: SUPPLIER_B
        model: gpt-5.6-sol
        weight: 3
```

上例是确定性 fixture 的有效最小形状，`.invalid` 地址不会被访问，也不声明任何真实供应商能力。测试加载后把 `base` 替换为环回服务器，并在测试存储中写入具名合成凭据；`key_env` 从不读取。它也不读取用户配置、数据库凭据或环境中的供应商密钥。真实供应商能力只能来自实际配置和成功证据。若要证明两个真实供应商都支持同一模型，必须在仓库根目录的 `config_provider.yaml` 中分别配置该模型和各自的 `key_env`，再让 `weighted_scenarios.deployments` 引用两个不同的 provider。

## 共享 schema 约束

共享 schema 的根字段为 `version`、`providers` 和 `weighted_scenarios`。`version` 必须为 1。每个 provider 的 `id` 必须是唯一的大写环境变量 stem；`credential_name`、`key_env`、`base`、`protocol` 和 `models` 必须完整，其中 `key_env` 是合法环境变量名，`base` 是不含凭据、query 或 fragment 的 HTTP(S) URL，`protocol` 只能是 `openai` 或 `anthropic`，模型列表不能为空且各项必须是去除首尾空白后的非空字符串。

每个 weighted scenario 的 `name` 必须是唯一的简单标识符，`model_name` 非空，`requests` 为 1–100，并且至少包含两个 deployment。deployment 的 `id` 在所有场景中全局唯一，`provider` 必须存在，`model` 必须属于该 provider，用于加权场景的 provider 必须采用 OpenAI 协议，`weight` 不得为负。场景总权重必须大于零；请求数必须包含约分后权重周期的整数倍。周期通过最大公约数约分并做溢出保护。

`internal/providerconfig.Load` 只接受一个 YAML 文档并拒绝未知字段。live metadata 的 JSON 解码同样拒绝未知字段和额外 JSON 值，解析错误不会回显输入内容。`weighted_live_metadata_test.go` 验证畸形 JSON、未知字段、无效版本或配置、全零权重和不完整周期均在读取凭据、建立可能产生费用的 live harness 之前失败；超大权重的精确预期次数通过约分和 `uint64` 计算，避免 `int` 乘法溢出。

## Fixture 与辅助函数契约

`internal/regression/multi_supplier_test.go` 负责建立供应商 A/B 的独立上游、部署清单、租户链和调用记录器。fixture 保留 provider、deployment、公开模型与上游模型之间的映射；上游根据请求认证解析供应商身份，记录解析后的身份、请求顺序、模型和注入状态码，不在调用记录中保存原始凭据值。辅助函数通过正式 HTTP 管理面写入路由设置与模板，通过正式推理入口发请求，并在需要持久化断言前刷新异步日志。

部署有两条稳定 ID 路径。配置文件构建的部署把 ID 放在 `litellm_params.deployment_id`；控制台 `/model/new` 创建并存入数据库的部署把行 ID 放在 `model_info.id`。`router.WeightID` 和 `router.CooldownID` 必须识别这两条路径，才能让 endpoint、credential 和上游模型完全相同的两行仍拥有独立权重、响应亲和、冷却、指标与账务身份。数据库专项故意不注入配置专用的 `deployment_id`，防止配置路径掩盖数据库行碰撞。

数据库部署以 `model_info.id` 作为独立的运行身份，配置部署可提供 `litellm_params.deployment_id`。运行状态按部署身份隔离；模板不读取已退休的 `model_info.deployment_id` 字段。

普通非流式成功调用逐次核对响应归属的 `deployment_id`、调用 ID、价格日志中的最终 token 数、snapshot 数量×费率和 user/key/project/team/organization 五层费用。并发流式场景不能从响应正文逐次归属部署，因此核对供应商请求聚合、每个调用的最终日志 token 数、总价格、成功日志数和五层累计。失败场景同时核对尝试顺序、未成功请求不结算，以及最终成功只产生一笔账。并发权重只按最终完整周期聚合判断，不依赖请求完成顺序。冷却用例必须连接 Redis；缺少 Redis 时该专项明确 skip，不能视为已验收。

Redis 花费队列是进程外共享状态，而各 harness 的 PostgreSQL schema 独立。harness 清理必须先执行 `server.Close`，等待仍在运行的流式 handler 返回；随后在 schema 仍存在时调用 `FlushSpend` 排空该 harness 的最终花费记录，最后调用 `Live.Close` 关闭 Redis 客户端。顺序颠倒会让迟到记录留在共享队列中，被下一个 harness 写入自己的 schema，表现为预期 20 条却读到 21 条。这个顺序只能隔离同一测试进程内依次关闭的 harness；多个测试进程不得共享 Redis。每个同时运行的 regression runner 都必须使用自己的专用 Redis 实例和 `XHUB_REGRESSION_REDIS_URL`。

## 场景矩阵

| 顶层测试 | 覆盖行为 | 核心证据 |
| --- | --- | --- |
| `TestMultiSupplierConfiguredWeights` | 7:3、70:30、1:1、10:0、三部署 7:2:1，以及同一 provider 的重复部署 | 每个场景最终完整周期聚合的部署计数；缩放权重产生相同比例；零权重部署从候选集中排除；部署身份不因 provider 相同而合并 |
| `TestMultiSupplierTemplateWeights` | 模板中的 list/map 写法、部分覆盖继承、未指定时默认权重 1 | 保存后的模板、effective 配置和实际部署计数一致 |
| `TestMultiSupplierModelRulesAndScopeIsolation` | team/key 覆盖、另一模型、根默认、模板热编辑与清空 | key→team→更高层的整份配置来源正确；一个模型或作用域的更新不污染另一个 |
| `TestMultiSupplierRetryAndBilling` | 500、429、普通 400 和所有部署失败 | 可重试错误按规则转移，400 终止，成功只记一笔，全部失败不收费 |
| `TestMultiSupplierZeroWeightExcludesFailover` | 零权重部署在首选失败后仍不作为回退 | 零权重上游请求数保持为零 |
| `TestMultiSupplierAllZeroRejects` | 所有候选权重均为零 | 配置或请求被明确拒绝，不任意选择部署 |
| `TestMultiSupplierStreamingAndConcurrentWeights` | 流式与普通响应各并发 20 次 | 两种模式的供应商请求总数最终均为 A=14、B=6；流式检查完成事件、内容、最终 token 数、价格总和、日志数量和五层累计，普通响应另逐次检查 deployment 归属及 snapshot 的数量×费率 |
| `TestMultiSupplierResponseAffinity` | 后续请求携带前一响应标识 | 后续请求钉住已成功部署，归属来自提交的响应亲和信息 |
| `TestMultiSupplierDatabaseDuplicateDeployments` | 通过 `/model/new` 创建两个共享 endpoint、credential 和 `gpt-5.6-sol` 上游模型的数据库部署；仅 `model_info.id`、7:3 权重和 1×/2×价格不同 | 20 次为 14:6；第二行响应可继续钉住第二行且按其价格结算；禁用 A 后只选 B；再禁用 B 后拒绝请求、无上游调用且五层费用不变 |
| `TestMultiSupplierCooldownIsolation` | supplier A 的部署进入冷却，supplier B 的部署继续可用 | Redis 中 A 的稳定 `deployment_id` 有 TTL、B 的 key 不存在；冷却期间只选择 B，清除 A 的冷却后恢复 7:3 |

健康、无粘性、静态权重且没有重试或冷却变化时，确定性 smooth weighted round robin 在完整约分周期的整数倍上给出精确总数。fixture 对 7:3、70:30 和三部署 7:2:1 都在最终 20 次聚合后断言计数；它不要求前 10 次切片各自满足同一比例。1:1 和 10:0 场景在最终 10 次聚合后断言。该保证不意味着任意请求前缀都严格等于比例。响应粘性会固定后续请求；重试会增加尝试次数；冷却会改变候选集。因此这些场景分别断言成功归属、上游尝试和结算，不能把成功份额简单解释为原始静态权重。

## 与真实供应商用例的边界

`internal/regression/weighted_live_test.go` 的 `TestLiveConfiguredWeightedRouting` 读取 `scripts/with-live-vendors.py` 写入的 `E2E_PROVIDER_METADATA`。当前仓库配置是 FENNO 的两个 `gpt-5.6-sol` 部署，以及 QINIU 的 `moonshotai/kimi-k2.5`；它能证明同一真实连接下两个部署的完整权重周期，但不能证明两个不同真实供应商都支持 `gpt-5.6-sol`。文档和报告不得把这项能力补写出来。

真实用例逐次保存 call ID、deployment、provider、token 和费用，并汇总部署次数与费用。账务证据必须把响应费用头、持久化价格日志（数量、费率与快照）和 user/key/project/team/organization 五层累计连起来。外部供应商不能稳定制造 400、429、500、冷却或精确并发交错，因此这些保证由本地确定性用例承担。

## 已验证结果（2026-10-09）

PostgreSQL 与专用 Redis 下的完整严格回归通过，用时 133.907 秒；8 个 live 用例因未启用真实供应商而明确 skip，未发起外部付费调用。10 个 `TestMultiSupplier*` 顶层测试全部通过，包括数据库重复部署。gateway、dataplane、router、prefs 和 providerconfig 单元测试套件通过。本次记录不把尚未完成的最终 race 复跑或未执行的真实供应商用例列为通过。

## 运行与判定

```bash
go test ./internal/providerconfig ./e2e/providerconfig -count=1
XHUB_REGRESSION_STRICT=1 go test ./internal/regression -run '^TestMultiSupplier' -count=1 -v
python3 scripts/with-live-vendors.py bash scripts/regression.sh --live -v '^TestLiveConfiguredWeightedRouting$'
```

第二条命令只选择本专项的顶层测试；数据库仍按回归 harness 的要求准备，冷却场景另需 Redis。该 Redis 必须专属于本次 runner；不要让同时运行的其他 `go test`、回归脚本或 E2E 进程使用同一个 Redis URL。第三条命令按 `config_provider.yaml` 选择真实供应商并从 `key_env` 或显式配置的凭据来源取密钥，密钥不得进入 YAML、测试输出或证据文件。

验收报告记录完整命令、代码版本、pass/fail/skip、数据库与 Redis 状态，以及每个 deployment 的成功数和尝试数。真实场景还要记录非敏感 provider/model 标识、逐次 call ID、价格日志核对和五层账务增量。只比较完整权重周期；发生重试、粘性或冷却时说明候选集为何变化。

## 已知覆盖空白

本专项不证明未配置供应商的真实能力，不覆盖所有供应商协议版本，也不保证任意请求前缀或任意并发完成顺序的比例。真实跨供应商同模型只有在配置了至少两个确实支持该模型的 provider 并成功跑完 live 场景后才能声称通过。网络故障、供应商限流和真实价格变更需保留证据并与产品逻辑失败区分。
