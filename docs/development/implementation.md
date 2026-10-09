# XHub 全功能实现说明

[文档首页](../README.zh-CN.md) · [开发指南](README.md) · [接口参考](api-reference.md) · [配置与数据](configuration.md) · [回归方案](regression.md)

本文按用户功能说明入口、数据流、约束、失败行为与验收方法。包的导出函数、源码文件和具体测试名称在各级 README；本说明把跨包实现串起来。这里只描述当前实现，不把可保存配置、路径登记或目录模型等价为完整运行能力。

## 功能导航

- [01 启动、配置与健康检查](#01-启动、配置与健康检查)
- [02 登录、退出、bootstrap 与会话失效](#02-登录、退出、bootstrap-与会话失效)
- [03 组织、团队、成员、项目与角色](#03-组织、团队、成员、项目与角色)
- [04 个人密钥与服务密钥](#04-个人密钥与服务密钥)
- [05 模型部署、公开名称与暂停状态](#05-模型部署、公开名称与暂停状态)
- [06 供应商能力、命名凭据与官方传输](#06-供应商能力、命名凭据与官方传输)
- [07 路由模板生命周期与继承](#07-路由模板生命周期与继承)
- [08 策略、权重、重试、超时与冷却](#08-策略、权重、重试、超时与冷却)
- [09 普通推理与协议转换](#09-普通推理与协议转换)
- [10 SSE、断流与失败日志](#10-sse、断流与失败日志)
- [11 完整响应缓存与会话粘性](#11-完整响应缓存与会话粘性)
- [12 预算、模型范围、RPM 与 TPM](#12-预算、模型范围、rpm-与-tpm)
- [13 关键词护栏与上游扩展](#13-关键词护栏与上游扩展)
- [14 价格目录、人工费率与时段](#14-价格目录、人工费率与时段)
- [15 用量归一化与价格快照](#15-用量归一化与价格快照)
- [16 Redis 入队、事务落库与幂等](#16-redis-入队、事务落库与幂等)
- [17 日志、会话、活动与费用分析](#17-日志、会话、活动与费用分析)
- [18 官方视频任务、创建与轮询](#18-官方视频任务、创建与轮询)
- [19 本地家族资源、兼容路由与已移除功能](#19-本地家族资源、兼容路由与已移除功能)
- [20 测试结构、验收证据与维护](#20-测试结构、验收证据与维护)

## 01 启动、配置与健康检查

启动入口把 YAML 配置解析为 Config，打开框架存储和身份库，初始化内置供应商，再通过 gateway.New 组合模块。管理员只在不存在时播种；修改 YAML 初始密码不会覆盖数据库密码。未选择模板时使用平台默认文档，数据库同名设置覆盖 YAML。
健康检查分为进程存活与服务就绪；liveness 不能证明上游供应商、数据库或 Redis 均可工作。readiness/details 和 health/services 用于依赖排查。公开 UI 配置只供控制台启动发现，不应返回上游秘密。
配置支持 os.environ/NAME；运行环境只记录变量名。改变模型定义、启停或价格后，候选过滤和缓存摘要都需要跟随变更。

### 实现位置

- [config](../../internal/config/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)
- [store](../../internal/store/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)
- [live](../../internal/live/readme_cn.md)

### 验收要点

验收先检查无数据库的失败，再检查正常初始化、重复启动不覆盖账号，以及同名配置的数据库覆盖。配置值可读和真实请求已执行是两个独立断言。连接错误不应以 HTTP 200 空对象隐藏。

## 02 登录、退出、bootstrap 与会话失效

用户名密码登录建立普通用户或管理员会话；每次使用时核对当前账号与 session_version。退出使用相应 logout 路径，密码或角色改变应使旧版本会话失效。RequireUser/RequireAdmin 是控制面身份边界，不能用拥有管理员的推理 key 代替会话。
bootstrap 是创建第一名平台管理员的一次性流程。master key 只在明确应急动作集合中使用，不是公共推理凭据。bootstrap/status 用于展示是否需要首次设置；重复 bootstrap 不得再创建新的最高权限入口。
APIKeyFrom 按 x-litellm-api-key、Authorization、api-key、x-api-key 读取。多个头冲突时要按此优先级验证，不能让不同 handler 各自选另一把凭据。

### 实现位置

- [auth](../../internal/auth/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)

### 验收要点

验证错误密码、空输入、账号禁用、旧会话、版本变更、master 推理拒绝、重复 bootstrap 和管理员 key 无管理权限。失效凭据是认证错误，数据库读取失败是依赖故障。

## 03 组织、团队、成员、项目与角色

资源结构是组织→团队→项目，用户通过成员关系获得团队范围。平台管理员管理全局；组织管理员和团队管理员只管理授权范围。角色、资源归属和可见范围从数据库加载，不能信任请求中填的 organization_id。
创建资源后需重新读取列表及详情；编辑父子归属、移动团队和成员角色会影响未来访问。历史 usage 保存旧归属，不从当前成员关系反推。服务密钥绑定的团队/项目需与资源层级一致。
列表先由 Scope 限定 SQL。空团队列表表示没有范围，只有明确 All 才是全局。用户搜索和排序不能扩大已授权集合。不可见对象可用 404 隐藏存在性；需要审计的操作必须走持久化审计。

### 实现位置

- [authz](../../internal/authz/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)
- [gateway/identity](../../internal/gateway/identity/readme_cn.md)

### 验收要点

使用两个组织，分别覆盖平台、组织、团队管理者、成员、无团队用户。正向断言可管理本范围，反向断言跨组织读取、写入、成员变更、密码重置和模板选择被拒绝。不要只测平台管理员。

## 04 个人密钥与服务密钥

个人 key 绑定用户及成员团队；服务 key 用 owner_type 表达不同所有权，创建需要对应管理权限。生成时返回 plaintext，后续正常列表详情只返回安全标识。轮换返回新密钥，旧密钥失效；禁用、恢复、过期和删除由每次认证重新读取。
输入包含 team_id/project_id、key_alias、预算、模型范围、生命周期和 route_template_id 等。解析后要校验项目在目标团队、用户在目标成员范围，不能通过改 owner_type 跨组织发 key。
批量编辑逐目标检查授权与模板可见性。每次推理把 key/user/team/project/org 链带入预算、日志和账务；不同作用域是同一调用的视图。

### 实现位置

- [gateway/keys](../../internal/gateway/keys/readme_cn.md)
- [auth](../../internal/auth/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)

### 验收要点

生成→实际调用→读取日志→编辑限制→拒绝调用→恢复→轮换→旧 key 拒绝→删除。plaintext 不应在普通读取、错误日志或费用报表出现。模板不可见时写操作不得部分生效。

## 05 模型部署、公开名称与暂停状态

model_name 是客户端公开名，litellm_params.model 是供应商接受的名字。同一个公开名可配置多部署，运行时在这些部署间排序。数据库部署覆盖 YAML，Info/Available/List 根据身份和当前状态呈现可见模型。
新增/编辑必须有公开名与上游名，命名凭据必须存在且协议匹配。endpoint_types 表示接受能力；transport 规定实际转发。已退休 auto_router/adaptive_router 前缀拒绝创建。
/model/disable 与 /model/enable 写 disabled 布尔状态；暂停同时影响 /v1/models、available 和实际候选。已存在粘性或响应缓存不能恢复已禁用部署。删除与暂停不同，暂停保留配置可恢复。

### 实现位置

- [gateway/models](../../internal/gateway/models/readme_cn.md)
- [config](../../internal/config/readme_cn.md)
- [router](../../internal/router/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)

### 验收要点

验证必填、错误凭据、不匹配协议、无效价格、公开名多部署、数据库覆盖、暂停后发现消失且不能拨号、恢复后可调用。跨权限用户不应看到未经允许的模型。

## 06 供应商能力、命名凭据与官方传输

Capability 将 chat、completion、embedding、image、video、audio、rerank、moderation 等入口操作收束；Transport 描述 adapted 或已登记官方路径。入口 Paths 用于展示，Ops 才是执行过滤依据。
能力只从 endpoint_types 读取，未配置默认 chat；显式未知类型拒绝。传输只从 transport 读取；未配置使用 adapted。新增能力不自动实现上游转换，新增 transport 不自动授予任何人访问。
命名凭据减少模型编辑中直接携带 api_key，发送时解析其供应商信息。配置 api_base 覆盖默认地址。provider/all 的空白导入保证 init 登记；openai 包当前占位，实际适配在 llm。

### 实现位置

- [provider](../../internal/provider/readme_cn.md)
- [provider/all](../../internal/provider/all/readme_cn.md)
- [llm](../../internal/llm/readme_cn.md)
- [gateway/models](../../internal/gateway/models/readme_cn.md)

### 验收要点

检查能力与 inferenceOp 一致，未知显式类型拒绝、不同能力模型不能互用、方法大写和动作存在、路径不被部署私自扩大。凭据隐藏和真实请求认证要分别测试。

## 07 路由模板生命周期与继承

模板是完整 router_settings 文档。创建时从当前平台默认播种一次；更新是完整替换，遗漏字段不从旧文档或父级补回。0、false、空列表、null 必须保持。平台默认本身不是模板表中的特殊行。
请求优先级为 key→team→organization→platform；第一份选中的模板整份生效。账号和项目不增加层。清空 route_template_id 表示继承父级。已选模板并发消失回平台；读取故障返回失败，不能静默改走其它部署。
目标资源写权限与模板读权限分别检查。Selection 在 mutation 前加载真实归属并授权，缺失/不可见拒绝。usage 入口查询各作用域引用，仍被引用拒绝删除。控制台来源展示和请求期 Resolve 必须使用同一链。

### 实现位置

- [gateway/identity](../../internal/gateway/identity/readme_cn.md)
- [gateway/templateauth](../../internal/gateway/templateauth/readme_cn.md)
- [gateway/prefs](../../internal/gateway/prefs/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)

### 验收要点

配置往返、播种一次、优先级、清空逐级继承、作用域编辑隔离、引用删除、普通 org/team/key 保存路径、跨组织拒绝和故障拒绝均需覆盖。真实模型下再核对选择→调用→五层计费。

## 08 策略、权重、重试、超时与冷却

num_retries 是每部署总尝试次数，包含首次，至少 1；timeout 是每次尝试时限，默认 60 秒。allowed_fails 默认 3，非正值关闭冷却；cooldown_time 非正时实际记录使用 60 秒。修改所选模板后下一请求读取新文档。
普通排序支持忙碌、延迟、费用、用量等策略；simple-shuffle 当前偏向最高权重。weighted-split 用平滑加权调度，模板覆盖只按 deployment_id 或 pricing_id 的稳定身份匹配；0 排除，负数和非有限值忽略。ApplyWeights 不修改共享原始参数。
每次尝试重新检查身份及限制。429/5xx 按策略重试，普通其它 4xx 不自动换部署。流输出后不重试。普通策略全冷却仍可尝试，split 无正权重开放候选为空。
fallbacks、context_window_fallbacks、content_policy_fallbacks、retry_policy、model_group_alias、stream_timeout 等当前主要是保存和往返；跨公开模型名回退没有完整执行能力。

### 实现位置

- [router](../../internal/router/readme_cn.md)
- [gateway/prefs](../../internal/gateway/prefs/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)
- [live](../../internal/live/readme_cn.md)

### 验收要点

用可编排上游核对每部署尝试次数、精确拨号顺序、400 终止、429/500 回退、零权重绝不拨号、超时热修改和真实 Redis 冷却。真实供应商无法稳定强制 500，这些故障用模拟上游验证。

## 09 普通推理与协议转换

Chat、Messages、Responses、Gemini 等入口识别为对应 op，能力过滤后由 llm 构造供应商请求。Completions、Embeddings、Audio 等有各自正文形状，不因同属 LLM 就可任意互转。
请求路径执行鉴权、范围与预算，再处理 guard、hooks、plugins、模板、排序和发送。上游参数过滤避免 proxy 专属字段泄漏。api_base 的 /v1 与路径拼接必须经协议测试。
每个响应设置独立 x-litellm-call-id，与费用及日志对应。协议错误体按 OpenAI/Anthropic/Gemini 入口写入；只判断非空响应会漏掉错误 op、usage 或模型名。

### 实现位置

- [gateway](../../internal/gateway/readme_cn.md)
- [gateway/family](../../internal/gateway/family/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)
- [llm](../../internal/llm/readme_cn.md)

### 验收要点

固定真实协议形状，验证模型名改写、头、参数、非流/流 usage、错误体、能力拒绝和不同入口一致计费。真实模型调用补上供应商实际接受请求和实际返回 usage 的证据。

## 10 SSE、断流与失败日志

stream.go 按 SSE 事件处理响应，usage 可能在 message、delta、response 或最后块，需要归并。明确零与字段缺失有不同意义。流已输出的字节无法撤回，后续错误写入日志而不能重写 HTTP 状态。
没有完整成功证据时不写成功缓存和成功粘性；不能在半条回答后接上另一部署的流。客户端断开、上游读取错误、超时与正常 [DONE]/终态结束需区分。
失败日志保留阶段、状态、调用 ID 和可用计量，不承诺涵盖供应商已收费的每一次失败尝试。完整供应商对账是另一个运营流程。

### 实现位置

- [dataplane](../../internal/dataplane/readme_cn.md)
- [httpx](../../internal/httpx/readme_cn.md)
- [gateway/usage](../../internal/gateway/usage/readme_cn.md)

### 验收要点

验证输出前错误可重试，输出后错误停止、只一次拨号且无成功缓存/钉，响应读取错误不能被吞掉。两次相同流请求有不同 callID，均达到上游，不命中完整响应缓存。

## 11 完整响应缓存与会话粘性

响应缓存是进程内完整答案 LRU，与供应商 prompt cache 不同。命中完整响应不再拨上游且本地新增费用为 0；prompt cache 仍调用供应商并按缓存 usage 计价。流式不进入完整响应缓存。
缓存摘要包括身份、op、模型、正文、部署配置、模板、会话和查询等上下文。改暂停或价格配置不能复用旧成功答案。Flush 清本地条目，不清账务。
会话与 previous_response_id 钉以模型、调用方、标识构成 SHA256 元组，成功后才提交，TTL 一小时。官方任务钉另有七天期限。Redis 不可用时本地回退不能保证跨实例/重启保留。钉只优先已入选候选。

### 实现位置

- [cache](../../internal/cache/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)
- [live](../../internal/live/readme_cn.md)

### 验收要点

同身份相同非流请求命中、不同身份/模板/op 不命中，失败不能命中，流不缓存。钉不能恢复禁用或零权重候选；同 session 的不同租户不能共享供应商状态。

## 12 预算、模型范围、RPM 与 TPM

身份链逐层收窄模型范围，模板只选择执行参数，不能扩大允许模型或预算。预算分别观察用户/key/project/team/org 视图，任一达到上限在拨号前拒绝。
Redis 热支出补充尚未落库事件，以降低队列延迟引起的预算漏判。RPM/TPM 是运行限制，持久总账是另一个维度；禁用 Redis 时共享限额能力按实现降级。
目前没有预算预占，多个并发请求、长流或视频任务可能在各自通过检查后使累计超过上限。拒绝用例要证明没有上游调用、没有成功日志、没有费用变动。

### 实现位置

- [authz](../../internal/authz/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)
- [live](../../internal/live/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)

### 验收要点

逐层独立设小预算再调用；模型名单交集、wildcard/空范围、服务 key、过期和禁用均验证。RPM/TPM 在真实 Redis 下验收，并把并发无预占限制记录在运行文档。

## 13 关键词护栏与上游扩展

护栏配置按 chat pre-call 执行，block 在拨号前拒绝，redact 改实际发送正文。读取规则失败不应关闭规则。官方 bypass 暂不走普通聊天护栏。
Hooks 只提供进程内在途计数，Begin 的释放函数幂等，所有结束路径都 defer 释放。Plugin 注册命名扩展，按顺序 BeforeUpstream，首个拒绝停止后续，适用头保留。
扩展不是替代授权器，不具备任意动态脚本安装协议。运行日志、护栏结果、审计和用量日志有不同保留目标。

### 实现位置

- [gateway/guard](../../internal/gateway/guard/readme_cn.md)
- [hooks](../../internal/hooks/readme_cn.md)
- [plugin](../../internal/plugin/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)

### 验收要点

观察被拒绝调用零拨号、redact 上游正文、配置故障拒绝、插件顺序及头、nil/重复登记拒绝、在途释放恢复。不要仅验证本地规则函数返回 true。

## 14 价格目录、人工费率与时段

价格来源包括内置公开目录、供应商贡献、外部 feed 和持久覆盖。部署价格优先，再按上游/公开名寻找目录。catalog 定价需 base_model；manual 需明确有效价，明确 0 表示零价，缺失不表示免费。
rates 支持 measure/side/variant/window/unit_size/usd。usd 是基础单位价格，计算 quantity×usd。展示每百万 token 时转换一次，不能再除 unit_size。peak/offpeak 按 Asia/Shanghai 和节假日日历，通常采用调用开始时刻。
更新目录不重写历史费用。来源刷新、覆盖 reset 与调度应有状态可查；目录无可用价格的模型必须在操作流程提示需配置。

### 实现位置

- [catalog](../../internal/catalog/readme_cn.md)
- [gateway/models](../../internal/gateway/models/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)

### 验收要点

验证输入输出、缓存、图片/秒/查询、多时段、零值/null/缺失、非法单位、base_model 无价、人工价以及刷新失败。真实模型检查所用费率可追溯到快照。

## 15 用量归一化与价格快照

上游 usage 优先，缺失时按适用路径估算，明确 0 不用估算覆盖。OpenAI prompt_tokens_details.cached_tokens 是输入子集；Anthropic cache_read_input_tokens/cache_creation_input_tokens 加入总输入。不同形状不能共享错误假样本。
Charge.Applied 保存实际数量、单价、side/window/variant 及 fallback，price_snapshot 随记录落库。历史详情以 snapshot 为准；旧记录重算明确标注 recomputed，不混作当时价。
费用头、日志 spend、五层新增费用应同值。浮点金额没有固定精度账本保证，测试使用合理容差。成功费用无法自动核准所有断流或失败尝试的上游成本。

### 实现位置

- [catalog](../../internal/catalog/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)

### 验收要点

数量×单价合计等于响应费用，输入/缓存只计一次，输出等于实测数量，快照价在改价后保持。使用 live oracle 独立解释字段存在性，避免测试重复错误实现。

## 16 Redis 入队、事务落库与幂等

EnqueueSpend 原子登记 request_id、热支出和队列；重复请求身份不二次入队。Flush 读取批次→IAM.RecordUsage 事务→AckFlushed，确认必须在 commit 之后。
PostgreSQL 唯一 request_id 与冲突处理保证新增事件才增加聚合；整批无效/失败回滚。Redis Ack 失败后重读不二次加数据库费用，但热支出可能暂时重复显示。
Redis 去重集合不自动过期；队列可靠性依赖 Redis 部署持久化。没有跨存储事务、数据库 outbox 或完整多刷新者批次协调。RecordSpend 不返回 HTTP 持久化确认，依赖故障期间需监控及对账。

### 实现位置

- [live](../../internal/live/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)

### 验收要点

并发重复入队、重复数据库写、事务失败、commit 后 Ack 失败重读、确认时保留新入队费用和坏队首行为。实际 Redis 与 PostgreSQL 都必须运行，内存 fake 不能证明 Lua/事务原子性。

## 17 日志、会话、活动与费用分析

日志列表先应用授权 Scope，再分页，默认 50/最大 200。详情使用 request_id；会话以 key 优先、user 回退隔离，旧记录无身份时只能 session 分组。当前页汇总不是全部会话总量。
日志、活动保留零费用、失败和缓存事件，请求数/token 不以 spend>0 为条件。历史 provider 优先，防止改名后旧图表归属漂移。日期/时区偏移需要按接口约定解释；浏览器 west-positive offset 不能反向。
prompt logging 决定正文、头和响应是否存储，敏感字段写入时脱敏。特权正文读取必须审计。/spend/calculate 只是当前估价，未知价为 0 不意味着供应商免费。

### 实现位置

- [gateway/usage](../../internal/gateway/usage/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)
- [iam](../../internal/iam/readme_cn.md)

### 验收要点

同 session 多 callID、跨用户同名 session、跨页总数、零费用请求、UTC 日期边界、浏览器时区、provider 历史、正文隐私与审计失败拒绝。报表扫描上限不得称为完整总账。

## 18 官方视频任务、创建与轮询

创建接收任务 ID 并按调用方/transport/部署建立任务钉；查询不能把另一调用方同名任务发送给供应商。创建遇可重试状态与未知传输结果区别处理，后者不得盲目再创建。
每次 poll 独立 callID；成功终态且正 usage 用稳定 settlement identity 计费一次。等待、失败、零用量不推断生成费。第一次持久化失败后后续终态查询可重新结算；没有完整价格或未等终态时不能称真实费用已验收。

### 实现位置

- [provider/volcengine](../../internal/provider/volcengine/readme_cn.md)
- [dataplane](../../internal/dataplane/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)

### 验收要点

模拟创建重试、未知结果、跨租户查询、重复终态、首次落库失败和独立 callID。真实 BYPASS 测试需明确模型及供应商参数，创建/查询证据与终态账务证据分别报告。

## 19 本地家族资源、兼容路由与已移除功能

本地 Assistants/Threads 等元数据使用租户隔离，不构成供应商完整异步实现。files/batch/fine-tune/realtime 等未实现行为明确失败，不能通用 KV 返回 200 后认为功能可用。
路由目录、docs/testdata/catalog.json 是接线基线；显式 mount、catalog dispatch 和 removed surfaces 共同决定可访问入口。退休自动路由、旧通行功能和旧设置要在未知/已移除测试中拒绝，控制台不能继续宣传。
扩展接口需同步 handler、权限、业务实现、API 类型和前端文案。静态资产和截图只是使用帮助，不是验收记录。

### 实现位置

- [gateway/family](../../internal/gateway/family/readme_cn.md)
- [gateway](../../internal/gateway/readme_cn.md)
- [catalog](../../internal/catalog/readme_cn.md)

### 验收要点

枚举路由表面积以发现漏接线，同时为真实功能加行为断言。已移除路径必须拒绝，未知路径 JSON 404，元数据不同租户不可交叉读取。

## 20 测试结构、验收证据与维护

包级测试定位纯函数和失败分支；HTTP 回归覆盖真实网关/数据库接线；可编排上游验证故障；真实模型补充供应商请求及 usage；浏览器验证操作闭环。这些证据互相补充。
strict 阻止数据库不可达静默跳过，但不会禁止所有显式 Skip。Redis、双协议、视频等仍要按结果分类。计数、路由条数和一次全绿不能证明无限输入/并发组合完全覆盖。
每项验收写清资源、角色、动作、结果、无副作用边界和清理方式。新增目录/功能同步 READMEs、功能索引、接口和测试矩阵。临时测试日志放忽略目录，稳定规则写本文。

### 实现位置

- [regression](../../internal/regression/readme_cn.md)
- [testsupport](../../internal/testsupport/readme_cn.md)

### 验收要点

完整执行流程见 regression.md，浏览器见 e2e-regression.md。新功能需要可观察结果；无直接测试包和缺凭据场景保留为具体未验证项。
