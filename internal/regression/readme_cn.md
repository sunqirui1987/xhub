# regression

端到端回归测试。每个测试起一个真实网关，走真实 HTTP，用真实 PostgreSQL。上游默认是本地假服务器（`simulated`）。打开 live 之后，同一条业务链路可以再打一次真实供应商。

最近一次 `go test ./internal/regression/ -count=1` 通过。没设 `XHUB_REGRESSION_LIVE` 时，live 子测试跳过，不算失败。没设 `XHUB_REGRESSION_REDIS_URL` 时，冷却那一条跳过。

## 怎么跑

```bash
./scripts/regression.sh            # simulated，假上游，不花钱
./scripts/regression.sh --live     # 再跑 live，会花钱
./scripts/regression.sh -v TestBudget
make regression
make regression-live
```

数据库不可达时 `scripts/regression.sh` 直接失败，不会像 `go test ./...` 那样静默跳过。

需要：

- PostgreSQL，默认 `localhost:5433`（`docker compose up -d postgres`）
- 可选 Redis：`XHUB_REGRESSION_REDIS_URL`。配了才走真实热花费、限流和冷却
- live 还要 `XHUB_REGRESSION_LIVE=1`、`XHUB_REGRESSION_FENNO_KEY`、`XHUB_REGRESSION_QINIU_KEY`。密钥只从环境变量读

## 两种模式

`mode_test.go` 的 `runBoth` 给一条用例挂两个子测试。

| 子测试 | 什么时候跑 | 上游 |
| --- | --- | --- |
| `simulated` | 始终 | 本地假服务器。可以按模型注入 200 / 400 / 429 / 500，也可以卡住一条部署 |
| `live` | `XHUB_REGRESSION_LIVE=1` | fennoai 的 `gpt-5.6-sol`。断言看状态码、费用、日志和五层花费，不看假上游的拨号记录 |

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

没有 `Test*`。`runBoth` / `runSimulated` 挂 simulated 和 live 两个子测试。`openLiveChat` 用 fennoai 的 `gpt-5.6-sol` 建网关和五层租户。

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

`TestLiveFennoaiAndQiniuCallTheRealVendors`。没开 live 就整段跳过。开了之后三个子测试：

- fennoai chat：`fennoai/gpt-5.6-sol` 回 usage，并且被网关定价
- qiniu chat：`qiniu/deepseek-v3` 同样
- qiniu Seedance：经 Bypass 建一个真实任务

这三条证明网关发出去的形状是供应商认的。假上游证明不了这一点。业务链路的 live 子测试在各自文件里，用的是 `openLiveChat`，不在这个文件。

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

## 写新测试

- 从 `newHarness(t, chatDeployment("名字"))` 开始。要五层花费就用 `openScope`。
- 能在真实供应商上重复的业务断言用 `runBoth`。必须注入上游故障的用 `runSimulated`。
- 单价用 `testInputRate` / `testOutputRate`。
- 看上游之前先 `h.resetUpstream()`，再用 `h.upstreamCalls()` 或 `h.upstreamSince`。
- 从目录取数的接口用目录里确实存在的模型名，例如 `claude-4.1-opus`。
- 注释写中文，写清为什么值得测。
