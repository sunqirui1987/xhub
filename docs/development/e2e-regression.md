# E2E 回归流程与维护

## 保留真实数据集入口

`make testdata` 是唯一的数据生成入口，不提供 `test_data` 别名。配置清单为
`docs/testdata/real-acceptance/dataset.json`。启动前先校验供应商配置地址；构建阶段会清空指定的
PostgreSQL `xhub/public` 和 Redis DB 1，再生成并保留 3 个组织、9 个团队、
27 名成员、9 个项目、81 把成员个人密钥及 1 把管理员个人密钥（共 82 把），以及供应商、模型、路由和护栏配置。
管理员密钥归属于当前管理员本人，不绑定团队，单独保存在 `access.json` 的 `admin_key` 中。
该命令仅通过管理 API 构造数据，不查询供应商目录、不探测或运行模型，
不生成 Codex 会话、图片或视频，也不需要供应商密钥。凭据配置保留环境变量引用。

`make e2e` 从 `FENNO_AI_API_KEY` 和 `QINIU_API_KEY` 读取供应商密钥，
在上述基线上执行真实模型调用、浏览器、数据库计量核对及后台回归，会产生供应商费用。
模型阶段严格依次执行：GPT-5.6-sol → 七牛 `z-ai/glm-5` → `gpt-image-2` →
Ark Seedance → FAL Seedance → FAL Kling。前阶段失败立即停止后续付费模型调用。
已有基线缺少 GLM 时，验收只补充 GLM 部署及相关白名单。

GPT 和 GLM 均声明 Chat 与 Responses 入口，使用 Codex 类型的请求模拟器：
固定 `codex_cli_rs` 客户端标识和会话头，调用 `/v1/responses`，每轮只提交当前
用户输入，通过 `previous_response_id` 续接，显式 `store=false`。第一轮提供随机
项目标记，后两轮不再提供标记，必须从历史中正确回复。81 把成员密钥保留作数据基线；
共享验收按组织继承、团队继承、Fenno 单供应商、七牛单供应商各选一把代表，
GPT 共 4 个三轮会话（12 次请求）；GLM 另验证一个三轮会话。逐轮核对上游完整历史、
供应商粘滞、响应 ID、五级归属、usage、价格快照及金额。此模拟器验证请求协议，
没有启动 Codex CLI 或执行本地代码工具。

编排由 `verify_acceptance_models` 统一负责；会话请求与核账由
`codex_conversation` 负责；浏览器详情由 `codex_browser.cjs` 统一负责。完整会话
成功后保存检查点，复验仍会重新读取每轮真实账单，失效证据不会算通过。
网关日志记录恢复历史及护栏处理后的正文，客户端原始增量正文另存于检查点。
媒体阶段复验已保存的图片、任务及结算证据，避免无条件重复创建付费任务。
视频创建与终态使用同一个请求日志 ID：后台在原创建事件上更新 `completed`、
`task_settled`、响应和费用，保留原始 prompt。续跑按模型、协议及响应中的精确
Task ID 恢复该日志，同时兼容历史独立 `official-settlement:` 日志。
计量对账同时汇总 `success` 和 `completed`，浏览器按恢复 ID 核对输入与终态产物。
后续作用域、权限、护栏、缓存、权重及回退链继续进行独立业务回归。

默认验收按业务类型去重，同类型只选一个完整用例。Chat 按上述 4 种路由各测一次，
图片和三种视频协议各测一次；四种角色、五级预算、RPM/TPM、拦截/脱敏/放行、
缓存/权重/回退属于不同业务行为，继续保留。新增浏览器业务阶段使用独立临时库和
本地供应商夹具，选择 19 个完整流程：密钥、模型、成员、回退配置、管理设置、
用户编辑、正则护栏、XGo、聊天恢复、对比隔离、取消与移动端、错误详情、异步任务、
路由诊断、层级可见性，以及部署删除后的默认/继承模板清理、缓存清空后的计费 miss、
会话/密钥继承预览和完整路由巡检。文件与精确标题共同限定选择范围，
以 `scripts/e2e-real-dataset.py` 的 `run_business_browser` 为准。该阶段不会额外调用付费供应商。

运行前打印完整待测清单，每项显示编号、业务内容、开始与结果。例如先显示
「待测 001/026 entities 实体基线及团队成员」，随后显示该项的「开始」和「结果：通过」。
会话内部还打印「第 1/3 轮：建立上下文」及「第 2/3 轮：续接历史、核对上下文与独立账单」。

cases.md 是本轮人工核对表，cases.json 保存结构化结果；通过、失败、执行中、
未执行、检查点已复验分别统计。历史业务成功不会自动算成本轮通过；模型检查点
必须重新核对持久化证据才显示“检查点已复验”。失败后尚未执行的项目保留在清单中。
浏览器子用例也逐项打印编号、标题和结果，其独立报告是 business-browser-results.json。

产物位于 `.e2e/real-acceptance-current/`：`report.json` 保存会话与逐轮证据，
`browser-report.json` 和截图保存浏览器结果，`testdata.log` / `e2e.log` 保存阶段
输出（包含后台 regression 输出）。`access.json` 包含测试访问凭据，不分享。
保留基线用于人工查看；临时业务资源及本次服务结束时清理。

以下离线专项使用真实网关、数据库与浏览器，以及本地供应商夹具验证三轮上下文、
失败响应和日志页面，不证明外部 GPT、GLM、图片或视频供应商当前可用：

```bash
PYTHONPATH=e2e python3 -m unittest e2e/test_real_dataset.py e2e/test_codex_agent.py e2e/test_fake_upstream.py
bash scripts/regression.sh -v 'TestCodexAgentConversation|TestResponsesIncrementalContinuation|TestResponsesNativeToolContinuation'
bash scripts/e2e.sh codex-agent.spec.ts responses-continuation.spec.ts
```

视频日志恢复专项使用随机私有 schema 验证三类协议、旧格式、精确 Task ID、
非法正文、未结算及重复结算；浏览器验证按原创建 ID 重新打开完成结果与失败详情：

```bash
XHUB_MEDIA_RECOVERY_DATABASE_TEST=1 PYTHONPATH=e2e python3 -m unittest -v e2e/test_media_log_recovery.py
bash scripts/regression.sh -v 'TestSeedanceSettlementUsesMeasuredBandAndDeduplicates|TestFalSettlementUsesOutputSecondsAndDeduplicates'
bash scripts/e2e.sh task-request-logs.spec.ts
```

建数边界另由以下专项验证：真实管理接口创建完整层级，管理员登录后从组织及模型
页面读取数据、在个人虚拟密钥页面查看管理员密钥，并核对数据面日志为空。该浏览器用例需要空租户 schema，因此单独运行：

```bash
bash scripts/regression.sh -v TestTestdataSeedWithoutModelCalls
E2E_TESTDATA_SEED=1 bash scripts/e2e.sh testdata-seed.spec.ts
```

## 目标与验收标准


测试流程先于用例实现确定：

1. 检查 Node、Go、Python、数据库和 Chromium；为本次运行创建独立数据库 schema，选择独立端口和构建目录。
2. 构建生产控制台，启动模拟供应商、网关、控制台，等待健康检查。
3. 登录回归：空值、错误密码、正确密码、匿名访问、只读用户和密码重置。
4. 自动发现全部 `page.tsx`；逐页验证可见内容、重定向、空态与运行错误。另通过真实侧栏链接点击导航，通过真实页签点击页面分支。
5. 业务回归：模型与供应商、密钥生命周期、组织→团队→项目→用户、路由模板、Playground 与聊天、日志与用量、设置与保留的辅助表单。创建后检查列表，编辑后重新读取，删除后检查消失。
6. 失败回归：表单必填、取消不保存、搜索无结果、未授权访问、跨组织访问、只读写入拒绝、失效密钥、已移除路径。
7. 接口表面积巡检：逐条调用 catalog 基线，核对状态码和协议标头。这一层只证明路由接线，不宣称业务分支覆盖率。
8. 后端确定性回归：真实数据库与网关下执行权限、预算、计价、缓存、流式、重试、回退、护栏和模板配置测试。
9. 汇总测试结果、页面与导航覆盖，保存 HTML、JSON、JUnit、失败截图和 trace。失败返回非零；清理本次启动的进程和 schema。浏览器服务进程组提前退出时，外层脚本确认专属端口已空闲后补做 schema 和临时凭据清理。
10. 重复运行全部浏览器用例，验证环境和数据不会依赖上一次执行。

## 覆盖矩阵

| 范围 | 成功路径 | 失败或边界路径 | 用例入口 |
| --- | --- | --- | --- |
| 登录与会话 | 管理员、普通用户登录，初始密码和重置密码 | 空密码、错误密码、匿名跳转、只读写入 403 | `login.spec.ts`、`coverage.spec.ts`、`view-only.spec.ts` |
| 页面与导航 | 自动发现页面、点击侧栏和页签 | 页面运行错误、意外 5xx、新增页面漏测 | `coverage.spec.ts`、`pages.spec.ts`、`interactions.spec.ts` |
| 模型 | 配置供应商、添加、详情、连接测试、编辑、启停、删除 | 必填、取消、搜索无结果 | `wizards.spec.ts`、`writes.spec.ts`、`interactions.spec.ts` |
| 密钥 | 创建、显示密钥、编辑、再生成、禁用、恢复、删除、实际调用 | 旧密钥失效、禁用调用拒绝、删除取消 | `keys-playground.spec.ts`、`writes.spec.ts` |
| 身份与作用域 | 组织、团队、项目、用户、成员和各层级登录 | 组织间隔离、成员范围、只读权限 | `visibility-chain.spec.ts`、`wizards.spec.ts`、`view-only.spec.ts` |
| 调用与观测 | Playground、聊天、用量和日志 | 无数据、协议失败、预算与模型权限拒绝 | `keys-playground.spec.ts`、`coverage.spec.ts`、Go regression |
| 路由与配置 | 模板保存、重新打开、回退配置 | 跨组织模板拒绝、继承、清空、零权重 | `writes.spec.ts`、Go regression |
| 保留辅助页面 | 创建、更新、测试、删除、设置保存 | 已移除功能不再出现 | `column-writes.spec.ts`、`interactions.spec.ts` |
| XGo 自定义护栏 | 真实编译 `for text <- texts`，试跑拦截与放行，保存、刷新、编辑脱敏并重新读取源码 | 编译失败可见、无成功结果、保存 400 且列表无记录；旧 Python/外部 HTTP 模板不出现 | `xgo-guardrails.spec.ts` |
| 网关接线 | catalog 的所有登记路径 | 已移除接口、未知路径、协议错误 | `coverage.spec.ts`、`e2e/livesweep` |
| 数据面业务 | 协议、计价、预算、护栏、缓存、重试、回退 | 超预算、跨租户、断流、失败不计成功、价格快照 | `cmd/regression` |

“所有可能路径”以这里明确的场景和自动发现的当前页面为界。任意输入、并发时序、供应商故障的组合没有有限的完全穷举；新增业务分支必须补充具体断言，不能用盲目点击或 HTTP 200 代替逻辑验证。

## 可重复执行约定

### 模型与端点专项回归

运行 `make e2e-model-endpoints` 可重复验证本次模型列表及编辑问题。使用真实生产构建控制台、真实网关、PostgreSQL 和临时 Redis，供应商目录与推理响应使用本地模拟服务。这个入口不读取真实供应商密钥，不触发真实视频生成。默认 `make e2e` 按上文的保留真实数据集清单验收，两者的报告范围分别记录。

专项按以下顺序验证，浏览器失败后仍执行完整确定性后端回归：

1. 选择不同供应商表单，验证专属字段、默认值、认证草稿清理及凭据创建、编辑和删除。
2. 使用官方方舟凭据创建原生模型，验证保存、刷新、编辑、任务创建、查询和重复查询计费；本地上游核对认证和载荷。
3. 对已有 Chat 部署注入端点目录延迟与网络失败，校验原声明未清空，修改名称并保存，刷新后主动选择“所有模型”标签再检查持久化结果。目录失败场景只注入目录读取，保存及回读仍使用真实网关。
4. 点击 XGo 编辑器，以 JSON 输入执行真实编译器，校验放行、拦截、模板脱敏、保存与刷新回显；编译失败不能保存。非法 JSON、非字符串 texts 与旧 images 字段在客户端拒绝，加载请求示例后可恢复执行。
5. 复验密钥生命周期、模型连接/编辑/删除、护栏、团队成员、回退设置及两个相同部署的 7:3 配置保存；实际分流、预算、权限、计费、缓存、重试与协议语义由完整后端回归验证。
6. 写出 `.e2e/runs/<UTC时间>-models-<进程号>/summary.json`、`report.md`、`report.html`、实时日志、Playwright JSON/JUnit、失败截图及 trace；退出时删除临时 Redis 和浏览器私有 schema。跳过项单列，失败或结果缺失返回非零。

```bash
make e2e-model-endpoints
# 仅验证入口编排与报告完整性（不启动供应商/网关）：
python3 -m unittest scripts/test_e2e_summary.py scripts/test_e2e_model_endpoints.py
# 仅复验浏览器，不运行后端或生成统一汇总报告：
bash scripts/e2e.sh model-endpoints.spec.ts xgo-guardrails.spec.ts
```

手动端点用例在刷新重新编辑之后退出编辑器，实际点击“测试连接”确认页面反馈，再调用网关确认响应内容与 token 计量。通过响应 call_id 读取持久化账单，验证人工费率确实生效：8 个输入 token × 0.15 USD/百万 + 2 个输出 token × 0.60 USD/百万 = 0.0000024 USD。接口 200、页面提示与业务金额分别断言。

专项与完整供应商入口共享 acceptance.lock，避免并发覆盖产物；阶段的原始退出码写入报告。心跳结束时同时回收 sleep 子进程，避免命令完成后输出管道仍被持有。脚本契约测试覆盖成功退出 0、浏览器失败后继续后端、后端失败、缺失浏览器结果、Redis 启动失败以及锁占用拒绝。模拟阶段只验证脚本编排，产品业务仍由真实 Playwright 与后台 regression 验证。


- 浏览器环境使用专属端口、独立构建目录和随机 schema，不终止开发服务。
- 每个测试使用新的浏览器上下文；共享后端数据仅用于本次运行，各用例创建自己的业务资源。测试筛选运行也必须有效。
- 保持一个浏览器 worker，避免全局路由和日志设置互相覆盖。重试默认关闭，避免第一次失败被隐藏。
- 同工作区浏览器运行在端口检查、构建及报告清理前串行取得 `.e2e/browser.lock`；冲突默认最多等待 1800 秒，可用 E2E_BROWSER_LOCK_TIMEOUT 调整（0 为立即失败）。等待超时不移除其他运行的锁。真实验收的去重业务报告在释放锁前保存至其 business-browser/ 目录，避免后续排队运行覆盖证据。
- 通过可访问名称、标签和 test id 定位控件。以响应、元素状态和重新读取等待业务完成，不用固定睡眠确认成功。
- 密钥创建先读取真实会话的团队成员关系；多团队时等待选择框渲染并选择测试团队，不能用一次 `count()` 将尚未加载误判为无需选择。生命周期用例还验证 `/key/generate` 返回 200 且浏览器提交了非空 `team_id`，随后核对密钥展示与实际调用。
- 错误场景断言具体失败；成功场景不允许同时接受成功或失败提示。
- 真实聊天候选探测必须同时有非空最终回答和非零输入、输出 usage。报告仅记录 HTTP 状态、输出上限、finish_reason、正文及推理字符数，不保存回答正文。首轮使用 32 token；HTTP 200、正文为空且 finish_reason=length 时，在已有重试次数内把下一次输出上限提高至 1024。仅推理或仅计费仍算失败，扩容后仍失败则继续下一候选；502 等故障不触发扩容。
- 聊天模型选择限定在消息输入区域，避免账户菜单等其他 popover 的顺序变化影响点击。护栏成功用例使用“关键词 / 正则护栏”入口，创建后刷新，打开详情验证拦截，再编辑为脱敏并试跑核对替换结果。无效正则必须拒绝且不保存；已移除的供应商向导不能作为成功入口。列表参数每行一项，以数组提交；正则表达式内的逗号保持原样。
- 护栏试跑按名称查找不存在的规则时必须返回 404；试跑未保存规则必须显式提交完整内联配置。回归验证内联规则实际命中、数据库不新增规则、后续推理仍可放行，不能以不存在规则返回 allow 证明试跑成功。
- XGo 编辑器用例直接点击当前脚本的测试按钮，以真实编译器和执行器验证结果，并断言界面显示同一结果。保存后刷新重新打开，确认语言标记为 xgo、执行阶段为 pre_call、源码保持一致；编辑后的手机号脱敏必须实际返回替换文本。无效脚本的试跑失败与保存失败分别验证。
- make e2e 默认调用配置的真实供应商并产生费用。make e2e-offline 仅运行模拟供应商。密钥只从环境读取；显式设置 E2E_CREDENTIAL_SOURCE=database 时读取指定的已有凭据。

## 扩展规则

新增页面会进入自动发现集合。新增侧栏项会进入导航点击集合。新增业务操作应加入上面的矩阵并添加独立场景：写清前置资源、操作者、点击步骤、成功断言、失败断言和清理边界。

测试产物写入忽略目录，不提交截图、trace、运行日志或会话凭据。稳定的运行命令和故障排查写在本文，执行结果由报告提供。

## 隔离完整入口与供应商配置

执行顺序：加载并校验供应商 YAML → 创建独立 Redis → 构建控制台和 Go 程序 → 浏览器逐页点击与真实供应商推理 → 后端业务、协议及权重回归 → 生成报告 → 清理本次服务。每个阶段实时打印输出；编译或外部调用等待期间每 30 秒打印时间和日志位置。浏览器失败后仍执行后端并汇总失败证据。

```bash
# 环境中提供 config_provider.yaml 的 key_env 对应密钥后
make e2e-all
# 显式选择数据库中已保存的凭据，只读取配置指定的供应商
E2E_CREDENTIAL_SOURCE=database make e2e-all
# 免费确定性回归
make e2e-offline
# 只运行后端（仍按同一 YAML 选择真实供应商）
E2E_CREDENTIAL_SOURCE=database make regression-live
# 可筛选浏览器用例，报告明确显示筛选运行
E2E_CREDENTIAL_SOURCE=database bash scripts/e2e-all.sh weighted-routing.spec.ts
```



## 相同模型部署与分流验收


网关对 `traffic-split` 使用随机相对权重抽样。7:3 表示每次抽样的概率关系，有限次数不会保证恰好得到 7 和 3；确定性后端测试通过注入 `State.Draw` 检查区间边界，HTTP/live 样本只验证候选过滤、零权重排除、部署归属和计费证据。

TestLiveConfiguredWeightedRouting 每次向真实网关发送独立提示，经真实供应商返回后，从成功响应的 affinity 记录读取实际 deployment_id，再以 call_id 核对持久化费用、token、价格快照和数量×费率。最后核对部署请求数、成功账单数量及 user/key/project/team/organization 五层费用总增量。不能用返回的模型名区分两个同名部署。调用失败保留已完成证据，整项失败；不把错误或未执行请求算作成功。

浏览器 `weighted-routing.spec.ts` 创建两个相同连接部署，通过控件修改 7:3，保存、刷新、重新打开并检查两个独立部署身份。浏览器验证配置交互和持久化；实际随机分流与计费由后端回归验证。

真实分流还记录每个入站请求对应的上游 attempt 状态，断言完整样本中的上游调用证据。`max_attempts: 1` 将每个部署的尝试次数限制为一次，失败后仍可能切换下一部署；例如 A 的 DNS 失败后 B 成功，会改变成功归属概率。此时保留账单及 attempt 证据，不能仅凭最终 HTTP 200 或短样本计数推断权重算法。attempt 证据不记录请求头、URL、正文或原始异常，避免泄露供应商凭据。

预算、权限及模板的真实成功调用使用明确的“仅回复 ok”指令，并附带业务阶段标记。阶段标记用于区分请求，不让模型自由解释预算术语；模拟供应商请求保持原样。该约束减少无关生成，不能保证供应商延迟或可用性，真实超时仍按失败记录。

## 报告与诊断

每次完整入口产生 .e2e/runs/<UTC时间>-<进程号>/，.e2e/latest 指向最近一次。report.md 和 report.html 是最终报告；summary.json 包含所有浏览器/后端用例、pass/fail/skip、供应商模型、范围及分流证据。browser.log 与 backend.log 保留实时输出；browser/ 保存 Playwright JSON 和 JUnit；playwright-report/ 保存浏览器 HTML、截图和 trace；evidence/weighted-*.json 保存逐次 call_id、归属、token 和金额。缺少浏览器结果、后端未完成、失败或缺少配置权重证据均不能成为绿色报告。配置或凭据校验失败也会产生报告，并保留 preflight.log 中的原因；后端失败诊断直接列在报告中。

终端最后直接列出失败的浏览器和后端用例、简短错误，以及 HTML、Markdown、JSON 和完整日志路径。后端断言按 Go 的 RUN/CONT 事件关联到具体用例，不把最后一个 PASS 当成失败原因。`make: Error 1` 表示回归未通过，具体原因以这些断言和报告为准。供应商超时不自动算通过，也不自动重跑付费调用。

浏览器开始执行后缺少完整结果，报告会标记“执行未完成”，保留最后十行浏览器日志；这些进度记录不计入通过数，也不会误报成供应商配置检查失败。后端有执行日志但缺少套件结束记录时，同样标记未完成。报告只能确认结果未写出，不能据此断言进程被谁终止。

只复验真实模板继承与五层记账时，可以使用以下命令；它调用真实供应商，不能替代完整 E2E。真实模板初始超时显式为 90 秒，编辑为 75 秒后继续验证；当前没有同名或等价的确定性 Go 测试单独验证运行中超时热更新，这是待补的后台回归覆盖。与其他后端回归同时运行时，必须另外提供独立的 `XHUB_REGRESSION_REDIS_URL`。

```bash
E2E_CREDENTIAL_SOURCE=database python3 scripts/with-live-vendors.py \
  bash scripts/regression.sh --live -v '^TestLiveRouteTemplateSelectionAndBilling$'
```

接口巡检中的 route unsupported 是返回结构正确的 501 not_implemented，只能证明该接口明确拒绝未支持操作，不证明业务功能通过。报告保留这类清单。真正的 500、连接超时、未知 404 仍失败。价格目录刷新需要外部目录网络，供应商测试需要外网 DNS/TLS；失败保留具体原因，不自动降级为模拟供应商。媒体任务及同供应商双协议对比需要额外配置，跳过单列，不算通过。

`POST /guardrails/test_custom_code` 是脚本试跑接口，成功响应为 `success` 与 `result`，没有创建护栏的 id 或时间字段。巡检提交有效的 XGo 放行脚本，要求 `success: true` 且 `result.action: allow`；HTTP 200 的编译失败响应不能通过此项。脚本的拦截、修改、保存与失败分支由专门浏览器用例补充。

默认浏览器端口为 3100/4100/4110，可通过 E2E_UI_PORT、E2E_GW_PORT、E2E_UP_PORT 改为互不重复的空闲端口。Go 编译在健康检查前完成，避免冷编译消耗服务就绪超时。遇到占用锁要先确认原回归已经结束；不能删除运行中任务的锁。后端默认超时 1800s，可通过 XHUB_REGRESSION_TIMEOUT 调整；供应商单次调用仍有独立超时。
