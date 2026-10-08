# regression

端到端回归测试。每个测试起一个真实网关，走真实 HTTP，用真实 PostgreSQL。上游默认是本地假服务器（`simulated`）。打开 live 之后，同一条业务链路可以再打一次真实供应商。

没设 `XHUB_REGRESSION_LIVE` 时，live 子测试跳过，不算失败。没设 `XHUB_REGRESSION_REDIS_URL` 时，依赖 Redis 的冷却测试跳过；验证完整冷却链路时必须配置这个变量。

## 怎么跑

```bash
./scripts/regression.sh            # simulated，假上游，不花钱
./scripts/regression.sh --live     # 再跑 live，会花钱
./scripts/regression.sh -v TestBudget
./scripts/regression.sh -v 'Template|RouterSettings' # 路由模板和平台路由配置
make regression
make regression-live
```

数据库不可达时 `scripts/regression.sh` 直接失败，不会像 `go test ./...` 那样静默跳过。

需要：

- PostgreSQL，默认 `localhost:5433`（`docker compose up -d postgres`）
- 可选 Redis：`XHUB_REGRESSION_REDIS_URL`。配了才走真实热花费、限流和冷却
- live 要 `XHUB_REGRESSION_LIVE=1`，再加每家供应商的一组变量（见下一节）

## live 供应商：从环境变量发现，不写死在代码里

供应商会一直加，加一家不该需要改回归代码。所以每多一家，运维只要多设一组变量，套件自己就把它带上跑。写死的表每加一家都要动代码，于是"没覆盖到"和"没人改代码"变成同一件事。

变量名用 `<ID>` 作词干，大写，非字母数字换成下划线：

| 变量 | 必需 | 说明 |
| --- | --- | --- |
| `XHUB_REGRESSION_<ID>_KEY` | 是 | 密钥。缺了这家就跳过；有 KEY 没 BASE 直接失败，因为半配的供应商看起来和通过一样 |
| `XHUB_REGRESSION_<ID>_BASE` | 是 | chat 根地址。OpenAI 兼容的供应商要自带 `/v1` |
| `XHUB_REGRESSION_<ID>_MODELS` | 是 | 逗号分隔的模型名，供应商真正认的那个 |
| `XHUB_REGRESSION_<ID>_PROTOCOL` | 否 | `openai`（缺省）或 `anthropic`。决定走哪条上游路径、以及上游用哪套用量字段回话 |
| `XHUB_REGRESSION_<ID>_BYPASS_BASE` | 否 | Bypass 端点要的裸主机名，缺省由 BASE 去掉 `/v1` |
| `XHUB_REGRESSION_<ID>_BYPASS_MODEL` | 否 | 配了才跑 Bypass 建任务那条。端点类型因供应商而异，所以由配置点名 |
| `XHUB_REGRESSION_<ID>_BYPASS_ENDPOINT` | 否 | Bypass 部署的端点类型，缺省 `qiniu_contents_generation` |

例：

```bash
export XHUB_REGRESSION_LIVE=1
export XHUB_REGRESSION_QINIU_KEY=sk-...
export XHUB_REGRESSION_QINIU_BASE=https://api.qnaigc.com/v1
export XHUB_REGRESSION_QINIU_MODELS=deepseek-v3,kimi-k2
export XHUB_REGRESSION_FENNO_KEY=sk-...
export XHUB_REGRESSION_FENNO_BASE=https://api.fenno.ai
export XHUB_REGRESSION_FENNO_PROTOCOL=anthropic
export XHUB_REGRESSION_FENNO_MODELS=claude-haiku-4-5
```

密钥只从环境变量读，绝不写进仓库。供应商不可用（模型下线、网络抖动）会让用例红，这是有意的：一个连不上的 live 模型什么也证明不了，而静默跳过看起来和通过一模一样。模型被供应商下线时把它从 `<ID>_MODELS` 里去掉。

## 假上游的字段名照抄真实上游

本地假服务器回答的**字段名**是从在跑的供应商实际抓下来的，值仍然是构造的（`defaultReply` 固定，因为计费断言要拿它算钱）。

为什么必须这样：计量出错的方式就是字段读错，而字段读错在套件里能不能发生，取决于假上游回的字段像不像真的。从前假上游对 `/v1/messages` 也回 OpenAI 形状，于是 Anthropic 那一整套（`input_tokens` 只含未命中、`cache_read_input_tokens` 另算一笔）在套件里根本无从出现——那正是"账单少了一半而回归全绿"的原因。

| 上游 | 路径 | 用量字段 |
| --- | --- | --- |
| OpenAI 兼容 | `/v1/chat/completions` | `prompt_tokens`/`completion_tokens`/`total_tokens`，多两个 `*_details` 对象 |
| Anthropic | `/v1/messages` | `input_tokens`（**只含未命中**）、`cache_read_input_tokens`（另算一笔）、`cache_creation_input_tokens`、`cache_creation{}`、`output_tokens_details{}`、`service_tier`、`inference_geo` |

流式也分两套形状：Anthropic 是事件序列（`message_start` 把 usage 嵌在 `message` 里先报一次输入，`message_delta` 再平铺报最终输出），OpenAI 兼容是 `data:` 行、usage 只在最后一个 chunk 上。


## 两种模式

`mode_test.go` 的 `runBoth` 给一条用例挂两个子测试。

| 子测试 | 什么时候跑 | 上游 |
| --- | --- | --- |
| `simulated` | 始终 | 本地假服务器。可以按模型注入 200 / 400 / 429 / 500，也可以卡住一条部署。回答的字段名照抄真实上游 |
| `live` | `XHUB_REGRESSION_LIVE=1` | 环境里配的供应商，第一条模型。断言看状态码、费用、日志和五层花费，不看假上游的拨号记录 |

必须脚本化上游才能制造的故障（强制 500、429、卡住部署、未知策略）用 `runSimulated`。live 子测试仍会出现，但会跳过并写明原因。网关、数据库和记账在 simulated 里仍然是真的。

live 能重复同一条业务断言的用例：个人 / 团队 / 项目 / 组织 / 密钥额度、五层点名、模型名单、密钥花费重置、密码重置、幂等回放、坏请求、TPM。

## 一次请求按什么顺序分叉

文档和用例都按这个顺序。括号里是钉住它的文件。

1. 鉴权。无密钥、坏密钥、过期、停用（`permission_test.go`、`budget_chain_test.go`、`reset_test.go`）。
2. 正文。非法 JSON、缺 `model`（`more_cases_test.go`）。
3. 幂等键。非流式回放且不二次扣费；流式不回放（`more_cases_test.go`）。
4. 额度，由窄到宽点名：个人、密钥、项目、团队、组织。没设上限不是 0。成功的一笔费用同时加到这五张计数上（`budget_chain_test.go`、`budget_test.go`）。
5. 模型名单：团队 ∩ 项目 ∩ 密钥。空名单是继承（`model_limit_test.go`、`setup_test.go`）。
6. RPM / TPM。限流错误不能写成额度错误（`budget_test.go`、`more_cases_test.go`）。
7. 护栏在选部署之前，只覆盖对话。向量不跑护栏（`guardrail_test.go`、`more_cases_test.go`）。
8. 缓存只对非流式，命中不扣费但仍记一行（`consistency_test.go`、`interaction_test.go`）。
9. 路由策略选出第一条部署（`routing_test.go`、`router_settings_test.go`）。
10. 回退。5xx 和 429 换下一条；4xx 不换；没密钥的部署跳过；换部署前重新查额度；全失败是 502 且不扣费（`fallback_test.go`、`more_cases_test.go`）。
11. 成功后，响应 usage、计费头、日志、五层花费一致（`consistency_test.go`、`logs_test.go`）。

router-settings 里的 `fallbacks`、`context_window_fallbacks`、`content_policy_fallbacks`、`max_fallbacks` 能存能读，请求路径不读。`budget_duration` 没有周期清零。成员花费重置那条路由没有挂上。

## 文件

下面每个文件都说明它在套件里的位置、对外的入口，以及每个 `Test*` 证明什么。

### `doc.go`

包说明。没有测试。`liveModeEnabled` 读 `XHUB_REGRESSION_LIVE`，把这次是假上游还是真实供应商写进进程日志。

### `harness_test.go`

整套测试的底座。除了 `TestMain` 把 Gin 切到测试模式，这里没有业务用例。

`newHarness` 为每个测试起一个网关：独立 PostgreSQL schema，结束就删；随机端口；假供应商。没写 `api_base` 的部署被指到假供应商。配置写在 Go 里，不读 `configs/config.yaml`。

假供应商按路径回答，并记录每次请求的路径和正文。对话带固定 usage（提示 11、完成 5）。流式按 SSE 分块回，usage 在最后一帧。可以按上游模型名注入状态码（`scriptStatus`）、在回写前执行回调（`onUpstream`）、卡住一条部署（`holdUpstream`）。`h.live` 为真时表示上游是真实供应商，拨号断言改看费用和日志。

租户：`provision` 建组织、团队、用户和个人密钥。`openScope` 再加一个项目，密钥挂在项目上，五层花费才能一起动。额度用 `setUserBudget`、`setKeyBudget`、`setProjectBudget`、`setTeamBudget`、`setOrgBudget`。`spendOn` 直接写一条用量，只给「没设上限不是零」这种不需要真实调用的预置。

单价是 `testInputRate` / `testOutputRate`，不跟生成的价格目录走。

### `mode_test.go`

没有 `Test*`。`runBoth` / `runSimulated` 挂 simulated 和 live 两个子测试。`openLiveChat` 用环境里发现的第一家供应商的第一条模型建网关和五层租户——那些业务链路用例（额度、名单、密钥）要证明的是网关的记账和鉴权，与具体哪一家无关，所以不必每家都跑一遍；专门验计量的是 `pricing_live_test.go`。

### `chain_support_test.go`

没有 `Test*`。链路用例共用的断言。

`assertBilled` 走完一次成功调用，核对：有响应 id、上游模型序列（live 不核对假上游）、响应头金额、成功日志多一行且金额和 `project_id` 一致、组织 / 团队 / 项目 / 用户 / 密钥五张花费各增加同一笔。

`assertBudgetStop` 核对 429、正文点名那一层、不拨上游、五层花费和成功日志都不变。

`assertStopped` 用于停用、过期、名单拒绝：不是 2xx、不拨上游、花费不变。

### `setup_test.go`

供应商和第一把密钥。后面的额度、名单用例都假设「这把密钥本来就能调」。

| 测试 | 证明 |
| --- | --- |
| `TestProviderSetupAddsFennoaiAndQiniu` | fennoai 和 qiniu 能配上，目录读得到，从目录加进来的模型真的完成一次补全 |
| `TestScopedKeyCallsInference` | 发给租户的密钥能调推理 |
| `TestModelAllowListIsEnforced` | 限定了模型的密钥，调别的模型会被拒 |

### `endpoints_test.go`

断言上游收到的路径和正文，不只看状态码。

| 测试 | 证明 |
| --- | --- |
| `TestChatEndpointRewritesAndAnswers` | chat 到达上游，转发的模型名是调用方写的，回答是 OpenAI 形状 |
| `TestEmbeddingEndpointUsesItsOwnPath` | 向量走 `/v1/embeddings`，路径和 chat 不同 |
| `TestSeedanceBypassCreatesAndPollsATask` | 经网关路径建 Seedance 任务，正文改成供应商的内容生成形状，再查任务 |
| `TestABypassCannotBeFilledInFromTheConsole` | 部署上自填的路径表不会变成公开路由，请求不是 2xx，上游一次都没被拨到。已登记的 Bypass 由上一条覆盖 |

### `protocol_test.go`

同一个模型从不同调用方协议进来。

| 测试 | 证明 |
| --- | --- |
| `TestSameModelAnswersThreeChatProtocols` | OpenAI chat、Anthropic Messages、OpenAI Responses 三种形状各自正确 |
| `TestEveryProtocolIsBilledTheSameWay` | 三种协议扣的钱和记的 token 一样 |
| `TestStreamingAnswersArriveAndAreBilled` | 流式正文能到，并且照常计费 |
| `TestStreamingAndNonStreamingAgreeOnTokens` | 流式和非流式对同一批 token 记账一致 |
| `TestEmbeddingIsBilledFromItsOwnUsage` | 向量按自己的 usage 计费，不按对话的两个字段 |
| `TestUnknownPathIsNotFound` | 不认识的数据面路径是 404，不是空的 200 |

### `models_test.go`

模型怎么被列出来、加上、删掉、停用，以及价目表和端点目录是否对得上。

| 测试 | 证明 |
| --- | --- |
| `TestModelListShowsWhatTheCallerMayUse` | 列表只给调用方能用的模型 |
| `TestModelsOfEveryEndpointTypeAreListed` | 不只有 chat，别的端点类型也出现在列表里 |
| `TestModelAvailableCarriesThePriceFromTheCatalog` | `/model/available` 的卡片价格来自价格目录 |
| `TestModelAvailableLeavesUnknownPricesEmpty` | 目录里没有的模型，卡片价格是空，不会被编成 0 |
| `TestEveryDeclaredTransportExistsInTheCatalog` | 价目表里的 `endpoint_type` 要么是一条能力，要么是登记过的转发方式 |
| `TestTransportsAreRegisteredBypasses` | `/public/endpoints` 的转发方式都是 bypass，而且每条动作有方法和公开路径。协议适配不在这个列表里 |
| `TestPriceCatalogRatesArePerToken` | 非零单价落在每 token 的数量级。0 是合法的免费模型 |
| `TestAddingAModelMakesItCallable` | 管理接口加的模型立刻能调 |
| `TestDeletingAModelStopsIt` | 删掉之后调不动，并且从列表消失 |
| `TestConfigModelCannotBeDeletedFromTheConsole` | 配置文件里的模型不能从控制台删，重启还会读回来 |
| `TestBlockedModelIsRefused` | 停用后调不动，但仍留在表里；恢复后又能调 |
| `TestUnimplementedProviderNamesItsOwnProblem` | 供应商名不成立时报「这个供应商不支持」，不是凭据问题，而且不拨上游 |
| `TestMissingCredentialStillBlamesTheCredential` | 真的没配密钥时，仍然报凭据问题 |
| `TestUnknownModelIsRefused` | 未知模型的错误正文里带着那个名字 |

### `consistency_test.go`

一次调用之后，钱和 token 在各处是不是同一个数。

| 测试 | 证明 |
| --- | --- |
| `TestResponseUsageAndLogsAgree` | 响应 usage、`x-litellm-response-cost`、日志里的 token 和金额一致 |
| `TestUsageCountersMatchTheBill` | 额度判定读的累计花费，增加量等于这次日志扣费 |
| `TestCacheHitIsFreeAndLoggedAsOne` | 第二次相同调用由缓存应答，标成命中，不扣钱，但仍有一行日志 |
| `TestFailedCallIsLoggedButNotCharged` | 上游失败记一条观测，费用为 0 |
| `TestLogDetailMatchesTheRowInTheList` | 日志详情和列表里的同一行，字段一致 |

### `logs_test.go`

同一批调用在不同视图里加总。

| 测试 | 证明 |
| --- | --- |
| `TestEveryLogViewAgreesOnTheSameTraffic` | 日志页、按模型、按密钥的金额加总一致 |
| `TestCallIdIsTheSameEverywhereItAppears` | 同一个 call id 在各视图指向同一次调用 |
| `TestTenantSeesOnlyItsOwnLogs` | 租户看不见别的租户的日志 |
| `TestDailyActivityCountsTheCalls` | 按天聚合把当天的调用算进去 |
| `TestSpendCalculateUsesThePriceCatalog` | `/spend/calculate` 用价格目录。目录里没有的模型不能被编一个费率 |
| `TestPromptStorageFollowsTheSwitch` | 花费日志默认不留请求和回答正文。管理员打开开关后新日志留下正文，关掉后下一笔又不再留 |

### `budget_test.go`

额度链里没有单独展开的两件事，加上「一次调用就把上限顶满」。

| 测试 | 证明 |
| --- | --- |
| `TestBudgetAccumulatesAcrossCalls` | 上限拿累计花费比，不拿单次金额比。刚好够一次的上限放行这一次、拒绝下一次 |
| `TestRateLimitIsSeparateFromBudget` | `rpm_limit` 用尽是限流错误，正文里不是 budget |
| `TestUnlimitedScopeIsUnlimited` | 没设上限不是 0。预置大额花费之后仍然能调 |

完整的五层额度链路在 `budget_chain_test.go`。

### `budget_chain_test.go`

每条都是同一条链路，只是被设上限的那一层不同。先把上限留出余地，打一次成功调用，再把上限收到这次的实际花费上。这样 simulated 和 live 的 token 数不同也不影响断言。

成功时：五层各加同一笔，日志带 `project_id`。到顶时：429，正文是 `User` / `Key` / `Project` / `Team` / `Organization` 加上 `budget has been exceeded`，不拨上游，花费不变。再把上限抬高，下一次又成功。

| 测试 | 这一层之外还钉住 |
| --- | --- |
| `TestPersonalBudgetChain` | 停用个人会收回他的个人密钥，旧会话失效。重新登录后要新发一把密钥才能再调。旧密钥仍然不行 |
| `TestTeamBudgetChain` | 停用团队后同一把密钥被拒；恢复后又能调 |
| `TestProjectBudgetChain` | 成员看得见自己的项目，别的组织看不见。停用项目后拒绝，恢复后又能调。项目上限不能写成比团队更高，写失败后推理仍按原上限。删掉项目后，挂在它上面的密钥不再能调，也不拨上游 |
| `TestOrganizationBudgetChain` | 停用组织后拒绝，恢复后又能调 |
| `TestKeyBudgetChain` | 停用密钥立刻拒绝，解除后又能调。`duration=1s` 过期后拒绝，改成 `24h` 后又成功 |
| `TestBudgetNamesTheNarrowestExhaustedScope` | 个人和密钥同时到顶时点名 User。只剩密钥时点名 Key。项目和团队同时到顶时点名 Project。只剩团队时点名 Team。只剩组织时点名 Organization。全部放开后成功，只加一笔 |

这六条都有 simulated 和 live。

### `model_limit_test.go`

`TestModelLimitChain`。simulated 用两个本地模型 A、B：

1. 团队名单是 {A, B}，项目收成 {A}。调 A 成功并记账，调 B 在拨上游之前被拒，正文提到允许名单，没有成功日志。
2. 清空项目名单。B 继承团队，下一次就能调，不用换密钥。
3. 密钥名单只留 A。B 又被拒，A 仍成功。
4. 给项目写一个团队名单之外的模型，更新接口拒绝。随后的调用仍按收窄后的集合。

live 用真实模型名和另一个不存在的名字，证明名单内成功、名单外在拨上游之前被拒。

### `routing_test.go`

同一对外名下两条部署。必须脚本化上游或控制权重，所以 live 跳过。`TestRoutingStrategyChain` 里每种策略一个子测试：写入策略，调用成功，上游模型是策略选出的那条，日志金额和响应头一致。

| 子测试 / 测试 | 策略 | 谁先被选 |
| --- | --- | --- |
| `weight` | `simple-shuffle` | 权重最高 |
| `cost` | `lowest-cost` | `input_cost_per_token` 最低 |
| `latency` | `latency-based-routing` | `latency_ms` 最低 |
| `usage` | `usage-based-routing` | `tpm` 最低 |
| `tag` | `tag-based-routing` | 没打标签且不是第一条的部署被跳过。权重更高的无标签部署不会赢 |
| `TestLeastBusyChain` | `least-busy` | 第一条请求占住部署时，并发的下一条打到空闲的那条，并且记费 |
| `TestUnknownStrategyChain` | 不认识的名字 | 400，正文是 `unknown routing strategy`，不拨上游，花费不变。不会悄悄变成 `simple-shuffle` |
| `TestExactNameBeatsWildcard` | 精确名和 `openai/*` 同时存在 | 精确名打到精确部署。没有精确名的 `openai/wild-1` 被通配改写成 `wild-1`。通配改写后的 id 不在测试价目上，这条只核对上游模型名 |
| `TestSessionPinOverridesStrategy` | 会话钉 | 第一次按权重打到 A。改成会选 B 的最低成本之后，带同一个 `X-Session-Id` 的下一轮仍打 A。换一个会话才打 B |

### `pricing_test.go`

计费怎么算出来的。前面的用例核对"一次成功调用扣了多少钱"，这里核对**那个数怎么来的**：时段、缓存、按秒按张，以及事后能不能解释那一笔。每条都对应一个具体的漏钱方式，所以断言的是金额。

费率表是 `litellm_params` 上的一个键（`rates`），带四个维度：计费维度（token/张/秒/次）、侧（输入/输出/缓存读/缓存写）、变体、时段（高峰/空闲/不分）。它比扁平的每 token 两个字段表达力强，分时价和缓存价都在里面。

| 测试 | 证明 |
| --- | --- |
| `TestAWindowPricedModelIsChargedTheWindowItLandedIn` | 按高峰/空闲两档报价的模型，收的钱落在两档之一，账单上写的时段和实际收的那一档一致。绝对时段取决于用例跑在哪一分钟，所以这里钉的是"计费路径和时段判定是同一个答案"；日历本身由 `internal/catalog` 的单测覆盖 |
| `TestAPerSecondModelIsNotRecordedAsFree` | 价目表里没有每 token 的价、只有每秒的价的部署，按秒收。改动前这类调用取不到价，记一行零费用而且不报错 |
| `TestLogDetailExplainsTheChargeWithoutRecomputing` | 日志详情读调用当时存下来的费率（`source=snapshot`、`applied` 带单位数量单价），不是按今天的价目表重算 |
| `TestTheBreakdownSurvivesAPriceChange` | 同一次调用读两遍，中间把价改成十倍，数字不动。反面：改价之后新发生的调用按新价收，否则"没动"可能是因为计费根本没读价 |
| `TestAnUnpricedCallIsNotRecordedAsFree` | 目录里没有的模型不发计费头（发了等于说这次零元，而实际是不知道），日志行仍然在，金额是零，但不编造任何一侧的明细 |
| `TestACachedCallIsNotBilledAtTheInputRate` | 缓存读按缓存价收，不是输入价。日志里提示侧整体不小于缓存那一行，否则控制台减完会显示负数 |

### `pricing_units_test.go`

前面那个文件钉的是"这个数怎么来的"，用 `litellm_params.rates` 这一种写法。这里换角度看两件没覆盖的事：**控制台真正写出来的价格形状**，以及**目录里每一条模型、每一个它报价了的计费维度**。

| 测试 | 证明 |
| --- | --- |
| `TestConsolePriceFormIsBilledForEveryDimension` | 逐维度覆盖控制台的单价形状（token、缓存读、按秒、按张、按次各一条真实的部署 + 真实的调用）。价格只写在扁平字段上，也就是从界面加模型时会写进去的那几个键。每条断言金额、`source=snapshot`、账单里出现该维度、以及明细加起来等于实际扣的钱 |
| `TestTheFlatPeakFieldIsActuallyRead` | 控制台那两组单价格子里的高峰那一组真的被读了。改动前计费只读基础价，高峰时段按空闲价收——那一格填了不读，少收一半。断言"落在两档之一，且和账单说的那一档一致" |
| `TestAnthropicShapedUsageIsBilledWhole` | 按 Anthropic 形状让上游改口（`input_tokens` 只含未命中、`cache_read_input_tokens` 另算一笔），断言收的钱 = 未命中 × 输入价 + 命中 × 缓存价，并且日志行的 `cached_tokens` 和收的钱说同一件事。走 `/v1/messages`，假上游按真实的 Anthropic 形状回 |
| `TestTheCatalogPricesEveryMeasureItQuotes` | **守卫**：遍历内置价目表里每一条模型，对它报价过的每一个（计量，侧）按该计量试算一次，要求都能算出钱。改动前按秒的 146 条费率一条都取不到、44 个模型的联网搜索被归成按秒、22 个只有 thinking/text 变体的模型整个算不出钱，而当时套件全绿 |

### `split_test.go`

一个对外名挂两条不同供应商的部署，按比例分流量。`proxy_models` 的 `model_name` 上没有唯一约束，所以这种部署本来就能建出来；缺的是"按比例选出第一条"。

改动前 `simple-shuffle` 不是随机的，是取权重最大的那条，而权重默认 1，于是永远命中第一条——配了比例也不生效。新的 `weighted-split` 是它自己的策略，`simple-shuffle` 的语义不变（另外六个别名共用它）。

| 测试 | 证明 |
| --- | --- |
| `TestWeightedSplitSendsTrafficToBothDeployments` | 7:3 的两条部署跑十次，正好 7 和 3，两条都收到流量 |
| `TestWeightedSplitDoesNotChangeSimpleShuffle` | 权重高的那条排在后面时，`simple-shuffle` 仍然选它 |
| `TestSplitStillBillsAndLogsEveryCall` | 分流的每一次调用都留下日志行、都有金额。按比例分流最容易出的错是某一条的用量没记上，那会安静地漏掉一半收入 |
| `TestSplitIsEvenWhenNoWeightsAreSet` | 谁都没配权重时五五开。两条同名部署并排放着、没写权重，意思是"两边都用" |


### `router_settings_test.go`

`TestRouterSettingsChain`。live 跳过，因为中间要把一条部署改成 500。

1. 成员改 `POST /config/update` 被拒绝，下一次调用仍走权重更高的部署。
2. 管理员写成 `lowest-cost`，下一次立刻打到便宜部署并记账。
3. `num_retries=2`，便宜部署回 500。上游先见到两次失败，再见到贵的那条并成功。五层花费只增加成功那一笔。
4. `fallbacks` 写入后，`GET /router/settings` 能读回来。下一次调用仍打原来的部署池，不改打另一个对外模型。

### `route_template_test.go`

覆盖模板选择的基本链路：未选择时使用平台默认、团队覆盖组织、会话请求使用团队模板、控制台展示生效来源、引用中的模板拒绝删除，以及创建密钥时保存选择。全部走真实管理接口和推理请求，使用本地假上游。

### `route_template_config_test.go`

把整份路由模板配置的保存、范围绑定和请求效果连起来。平台默认是全局 `router_settings` 文档；新建模板时复制平台值，更新模板时整份替换。生效优先级是密钥 → 团队 → 组织 → 平台，选中模板后不再逐字段混入上层配置。

| 测试 | 证明 |
| --- | --- |
| `TestRouteTemplateConfigurationRoundTrip` | 控制台字段目录中的每个字段都有读写样例，创建、详情、列表保留完整正文；更新整份替换，零、false、空数组和 null 不丢失。新增字段未补样例会失败 |
| `TestRouteTemplateSeedsOnceFromPlatformDefaults` | 新建时继承平台快照；后续平台编辑不改变已选模板，清空选择后使用最新平台值 |
| `TestRouteTemplatePrecedenceUsesOneWholeDocument` | 逐级选择和清空组织、团队、密钥模板，策略和重试次数一起生效；控制台来源与实际拨到的上游一致 |
| `TestRouteTemplateEditsApplyOnlyToSelectedScopes` | 修改模板后下一次请求立即使用新正文，未选择该模板的租户不受影响 |
| `TestRouteTemplateRetriesAndFailoverBillOnce` | 500/429 按模板次数重试后换部署，只扣成功一笔；400 不重试、不计费；全失败返回 502、不扣费 |
| `TestRouteTemplateWeightsDriveTraffic` | 编辑器列表和映射两种权重形状均覆盖；模板 3:7 覆盖部署 9:1，十次请求准确分成 3 和 7，逐次核对计费；清空后恢复平台策略 |
| `TestRouteTemplateTimeoutChangesWithoutRestart` | 平台或模板的请求超时修改立即生效；阻塞上游触发超时且不计费，抬高超时后恢复成功 |
| `TestRouteTemplateCooldownUsesSelectedThresholds` | 使用真实 Redis 验证模板失败阈值和冷却 TTL，后续跳过冷却部署；清空选择后共享部署冷却仍有效 |
| `TestRouteTemplateScopeFormsKeepAndClearSelections` | 组织、团队、密钥创建和更新接口都保存选择；省略字段保持，null/空串清空；实际请求同步恢复继承 |
| `TestRouteTemplateUsageAndDeletionCoverEveryScope` | 三种范围均计入引用数，删除返回 409 和完整引用列表；逐级解绑后可删除，随后读取为 404 |
| `TestRouteTemplatePermissionsRejectCrossOrganizationChanges` | 平台模板可见、其他组织模板不可见；跨组织选择和修改平台模板被拒绝，原配置与绑定保持 |

这里区分“字段能保存”和“字段参与请求”。策略、权重、重试、请求超时和冷却有请求效果断言；模型级 `fallbacks`、流式超时、重试策略、别名等高级字段只验证保存和读取，不能据此认定请求期已实现。模型级回退当前只存储，全部署失败仍返回 502。

这些测试不调用真实供应商。完整运行（含冷却）时，先准备专用 Redis，再执行：

```bash
XHUB_REGRESSION_REDIS_URL=redis://localhost:6379/0 \
  ./scripts/regression.sh -v 'Template|RouterSettings'
```

### `fallback_test.go`

`TestFallbackChain` 用控制台加的两条数据库部署，这样它们才能被单独停用。live 跳过。

1. 第一条 500、第二条 200。调用方 200，上游顺序是失败的然后成功的，只记成功那一笔。
2. 第二条改成 400。调用方拿到 400，没有继续往下试，花费不变。
3. 两条都 500。调用方 502，花费不变，日志有失败行且金额为 0。
4. 第一条回 500 之前把项目花费顶到上限。换部署前重新查额度，第二条不被拨到，正文点名 Project。
5. 停用第一条后流量走第二条并记账。两条都停用是 `model_paused`，不拨上游。
6. 护栏命中词表时，两条都不被拨到。

`TestCooldownSkipsADeployment`：`allowed_fails=1` 时，一次 500 后这条部署进入冷却，下一次只打另一条。没配 `XHUB_REGRESSION_REDIS_URL` 就跳过，不把没跑当成通过。

### `reset_test.go`

| 测试 | 证明 | 模式 |
| --- | --- | --- |
| `TestResetSpendChain` | 密钥额度用尽后 `POST /key/{id}/reset_spend` 把密钥花费归零，历史成功日志还在，审计有 `key.reset_spend`，然后又能调。`reset_to` 写成当前花费时仍因 Key 拒绝。项目仍到顶时，密钥归零后下一次因 Project 拒绝。抬高项目上限后恢复。别的组织的成员重置这把密钥会被拒，花费不变 | simulated 和 live |
| `TestPasswordResetChain` | `POST /user/set_password` 之后旧会话不能再访问 `/auth/me`。新密码登录后，原来的密钥还能推理 | simulated 和 live |

### `chain_test.go`

`TestRequestChain`。同一个租户把几条链路串起来，防止分开绿、合在一起抵消。live 跳过，因为中间要把便宜部署改成 500。

建五层并设上限 → `lowest-cost` 打到便宜部署 → 便宜部署 500 后回退到贵的那条，只记一笔 → 项目收到实际花费上，下一次因 Project 拒绝 → 重置密钥后仍因项目拒绝 → 抬高项目上限后恢复 → 项目名单收窄，名单外的模型被拒、名单内的成功 → 成功日志至少三行，最后一行的 `project_id` 是这个项目。

### `more_cases_test.go`

请求链上其余分叉。

| 测试 | 证明 | 模式 |
| --- | --- | --- |
| `TestIdempotencyReplaysWithoutASecondCharge` | 同一个 `Idempotency-Key` 第二次原样回放第一次的正文，花费只增加一笔。simulated 还核对上游只被拨了一次 | simulated 和 live |
| `TestStreamSkipsIdempotency` | 流式不进幂等缓冲。同一个键的两次流式调用都会拨上游 | 只 simulated |
| `TestMalformedRequestsNeverDial` | 非法 JSON 和缺 `model` 都是 400。simulated 核对上游 0 次 | simulated 和 live |
| `TestTPMLimitIsNotABudgetError` | `tpm_limit=100` 放行第一次（估算至少 96），拒绝第二次。正文是 `tpm_limit`，不是 budget。simulated 核对被拒的那次没有拨上游 | simulated 和 live |
| `TestUpstream429FailsOverAndAMissingKeyIsSkipped` | 429 和 500 一样换下一条，并且只记成功那笔。第一条部署没有密钥时跳过它，打到剩下那条 | 只 simulated |
| `TestGuardrailDoesNotCoverEmbeddings` | 对话护栏的词表不拦 `/v1/embeddings`。上游路径含 embedding | 只 simulated |
| `TestEachInferenceFamilyReachesItsOwnUpstreamPath` | completions、生图、审核、重排、语音各自打到自己的上游路径，而不是全被送去 chat | 只 simulated |

### `permission_test.go`

谁能看见谁。越界往往不是报错，而是多看见一条。

| 测试 | 证明 |
| --- | --- |
| `TestTenantIsolationAcrossOrganizations` | 一个组织看不见另一个组织的团队和密钥 |
| `TestMemberCannotReadAnotherTenant` | 越界读取按找不到拒绝，不泄漏那条记录存在 |
| `TestMemberCannotAdminister` | 普通成员改不了团队，也发不了别人的密钥 |
| `TestKeyLifecycleAndScope` | 签发、能用、能被看见、停用后立刻失效，列表里没有别的租户的密钥 |
| `TestKeyRegenerateInvalidatesTheOldSecret` | 重新生成后旧密钥立刻作废 |
| `TestUnauthenticatedIsRefused` | 不带身份时，管理面和数据面都拒绝 |
| `TestInvalidKeyIsRefused` | 乱编的密钥换不到任何东西 |
| `TestOrganizationAndTeamHierarchy` | 团队挂在指定组织下，成员加入后从团队视角看得见 |

### `guardrail_test.go`

| 测试 | 证明 |
| --- | --- |
| `TestGuardrailBlocksBeforeTheUpstreamIsDialed` | 词表命中是 400，上游 0 次。没命中的提示词放行，上游 1 次 |
| `TestGuardrailBlockIsLoggedButNotCharged` | 拦截记一条日志，不扣费 |
| `TestGuardrailOnlyAppliesToTheModelsItCovers` | 规则只拦它覆盖的模型 |
| `TestGuardrailTrialDoesNotChangeStorage` | `/guardrails/apply_guardrail` 试跑不把规则写进存储 |
| `TestGuardrailRedactRewritesInsteadOfBlocking` | 打码让调用通过，只改写命中的词 |

### `interaction_test.go`

单看每个子系统都对，凑在一起可能互相抵消。

| 测试 | 证明 |
| --- | --- |
| `TestGuardrailBlockDoesNotConsumeBudget` | 被护栏拦住的调用不占额度 |
| `TestCacheHitDoesNotConsumeBudget` | 缓存命中不推进额度 |
| `TestManagerActionsAreAudited` | 管理动作留下审计记录 |
| `TestChangingABudgetTakesEffectImmediately` | 改额度下一次调用就生效 |
| `TestBlockingAKeyTakesEffectImmediately` | 停用密钥下一次调用就生效 |
| `TestDeletingATeamStopsItsKeys` | 删掉团队后，它下面的密钥失效 |
| `TestModelRestrictionAndBudgetAreIndependent` | 名单拒绝和额度拒绝各报各的理由 |

### `live_test.go`

`TestLiveConfiguredVendorsAnswer`：环境里配的每一家供应商、每一条模型各发一次真实调用，要求回正数用量、并且网关按内置价目表把这次调用算出了非零金额。没开 live 或没配供应商就整段跳过。

`TestLiveBypassCreatesAndPollsARealTask`：只有配了 `<ID>_BYPASS_MODEL` 的供应商跑。经网关建一个真实任务再查回来，证明供应商返回的 id 就是网关能再查回去的 id。

### `pricing_live_test.go`

拿**真实模型和真实上游回答**验计量。假上游能证明网关自己的接线对，证明不了"网关读得懂供应商真正发回来的那个东西"——而计量出错恰恰在这里。

| 测试 | 证明 |
| --- | --- |
| `TestLiveRealModelsAreBilledFromTheCatalog` | 每条真实模型：上游回了正数用量、网关按内置价目表算出非零金额、账单留下了用到的费率、**费率里的数量等于上游报的数量**、数量 × 单价 = 收到的那笔钱、用量行里的 token 数也是真实的。部署上不写单价，所以价只可能来自价目表 |
| `TestLiveVendorUsageFieldsAreUnderstood` | 把真实回答里的用量字段名逐个列出来，要求网关的归一化认得它们，并把提示侧读成"整段提示"。新接一家用第三套拼法时会红在这里，而不是红在账单上 |
| `TestLiveStreamingIsBilledFromTheRealUsage` | 流式也读得到真实用量。断言读日志行而不是响应头——流式开始写正文后就不能再补计费头，这是有意的 |
| `TestLiveSameVendorViaEitherProtocolIsBilledAlike` | 同一家、同一个模型的两种协议被同等计费。两套字段名（`input_tokens` 与 `prompt_tokens`）指同一笔钱，单位价必须一致。只在同一 base 同一模型同时配了两种协议时跑 |

## 断言约定

额度按 `spent + hot >= ceiling`。上限之下的调用放行，哪怕这一次会花超。链路用例先成功调用一次，再把上限收到实际花费上，不靠猜 token 数。

只断言 200 不够。会核对上游正文或路径、拨了几次、日志里的钱和 token、五层花费是不是同一笔。

有意的行为写在测试注释里：配置文件里的模型删不掉；目录里没有的模型不编费率；流式开始写正文之后不能再补计费响应头；自填的 Bypass 路径表不是路由；模型级 fallbacks 只存不执行。

## 修过的缺陷

回归挖出来并且已经修好的问题。

### 失败原因被张冠李戴

`internal/dataplane/serve.go` 的终局失败原来把三种情况并成「没有上游密钥」：

| 真实原因 | 原来报的 |
| --- | --- |
| 供应商名写错 | 401，说没有 API key |
| 供应商无法为此编码请求 | 401，说没有 API key |
| 真的没配密钥 | 401，说没有 API key |

根因是 `lastProvider` 在供应商合法性检查之前就被赋值了。

现在分开：

- 没配密钥 → 401，仍说凭据问题
- 供应商不成立 → 400，正文点名那个供应商
- 供应商成立但编不出请求 → 400，正文点名供应商和编码错误

钉住它们的测试：`TestUnimplementedProviderNamesItsOwnProblem`、`TestMissingCredentialStillBlamesTheCredential`，以及 `internal/dataplane/failure_log_test.go` 的 `TestServeLogsBuildSkipAndTerminalAuth`。

### 失败的调用不记账

终局失败原来只留进程日志。现在和 400 一样记一条观测，状态在，token 和费用为 0。由 `TestFailedCallIsLoggedButNotCharged` 钉住。

### 计费维度整体取不到价（按秒、按张、按次）

由 `pricing_units_test.go` 的守卫挖出来，已经修好。三条都是同一种漏收：状态码 200、不报错、账单上是零或明显偏小。

| 原来 | 根因 | 现在 |
| --- | --- | --- |
| 按秒的 146 条费率一条都取不到 | 市场把限定词写进了变体名（`1080p_v_duration`），而查找链只会退到空变体 | 一侧只有变体价时取其中最便宜的一档，并在 `applied` 上标 `fallback`，让"选出来的"和"量出来的"能分开 |
| 44 个模型的联网搜索被归成按秒 | `web_search_req` 的单位名是 `second`，但计的是**次数** | 识别 `search` 限定词，按 `query` 计费；报秒数时不会误收搜索费 |
| 22 个只有 thinking/text/wiv 变体的模型算不出钱，3 个静默丢掉输入侧 | 同上：整侧只有变体价 | 同上。`hunter-alpha` 这类本来就免费（价是 0）的不受影响 |

守卫本身留在套件里：它遍历整张价目表，对每一个它报价了的（计量，侧）试算一次。价目表刷新后某个维度再次整体取不到价，它会红。

### 控制台填的价被绕过

控制台的单价表单一个格子一个字段，提交的是 `input_cost_per_token`、`output_cost_per_second` 这样的扁平键，不是一个 `rates` 数组。而计费路径对"部署自带价"走的是另一套算术，只读每 token 的两个字段。

结果是三件事一起发生：高峰那一格填了不读（高峰按空闲价收，少收一半）、缓存读被算进 input 又加进 total（带缓存的调用多收一遍）、按秒按张按次的价一律不存在（记一行零费用）。

现在扁平字段转成费率表之后走**同一段**计费算术（`catalog.RatesFromFlat`），没有第二份实现可以漂移。由 `TestConsolePriceFormIsBilledForEveryDimension`、`TestTheFlatPeakFieldIsActuallyRead` 钉住。

### Anthropic 形状的用量被少算一半

供应商对"输入 token"的定义不一致：OpenAI 的 `prompt_tokens` 是整段提示，Anthropic 的 `input_tokens` 只含未命中的那部分、缓存读另算一笔。用前者的读法读后者，800 个缓存 token 整个丢掉，而剩下的 200 个反而按缓存价收。

这套回归一直没发现，因为假上游对 `/v1/messages` 也回 OpenAI 形状——那一整类解析错在套件里无从发生。现在假上游的两套形状都照抄真实上游，`catalog.NormalizeUsage` 按"缓存计数写在哪里"区分两种语义（顶层字段是第二笔，嵌套在提示计数下的是子集）。由 `TestAnthropicShapedUsageIsBilledWhole` 和 `TestLiveVendorUsageFieldsAreUnderstood` 钉住。

## 写新测试

- 从 `newHarness(t, chatDeployment("名字"))` 开始。要五层花费就用 `openScope`。
- 能在真实供应商上重复的业务断言用 `runBoth`。必须注入上游故障的用 `runSimulated`。
- 单价用 `testInputRate` / `testOutputRate`。
- 看上游之前先 `h.resetUpstream()`，再用 `h.upstreamCalls()` 或 `h.upstreamSince`。
- 从目录取数的接口用目录里确实存在的模型名，例如 `claude-4.1-opus`。
- 假上游回答的字段名要照抄真实供应商。值随便构造，**字段名不能编**——字段形状错了，
  用例就测不到真实路径上会发生的错。新加一种回答形状时先对着真供应商抓一次。
- 要覆盖真实模型和真实上游回答的，放进 `pricing_live_test.go`，供应商从环境变量发现，
  不要在代码里写死任何一家的名字或地址。
- 注释写中文，写清为什么值得测。
