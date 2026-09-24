# Go 网关对齐 LiteLLM 1.102.0 的迁移重构计划

基线是 `/Users/sunqirui/Downloads/litellm-main` 的 LiteLLM **1.102.0**（`pyproject.toml` 的 `version = "1.102.0"`）。分母以 `docs/_inventory/catalog.json` 为准：`unique_http_routes` **779**，`provider_packages` **136**，以及下面第 6 节列出的全部 `data.*` 与 `mgmt.*` 家族。`docs/alignment-vs-litellm-1.102.0.md` 只作提示。它写「131 个供应商运行时返回 `provider_not_implemented`」，和当前代码不一致，本文不沿用那张表。

本文是能力对齐，不是目录对齐。客户端和已随仓库提供的控制台真正发出的 **HTTP 方法、路径、请求体、响应体** 要和 LiteLLM 1.102.0 一致。Go 进程内部怎么拆包，不必、也不许照着 `litellm/llms`、`litellm/proxy` 或 Prisma 模型铺目录。

779 条路由里，只靠 `catalogFallback` 变成非 404 的，不算对齐。供应商只靠 OpenAI 兼容 URL 转发的，也不算对齐。

## 1. 现状

进程今天用标准库 `net/http`，不用 Gin。

`cmd/gateway/main.go` 在配置和 SQLite 打开之后调用 `http.ListenAndServe(*addr, srv.Handler())`。`internal/server.New` 把 `Mux` 设为 `http.NewServeMux()`，先登记一批专用 `HandleFunc`，再把 `/` 交给 `catalogFallback`。

`go.mod` 的直接依赖是 `github.com/hashicorp/golang-lru/v2`、`golang.org/x/crypto`、`gopkg.in/yaml.v3`、`modernc.org/sqlite`。模块图里没有 Gin，没有 `github.com/openai/openai-go`，也没有 `google.golang.org/genai` 或其它 Gemini Go SDK。这三项都还没引入，后面的阶段才允许加。

上游调用在 `internal/llm` 里手写：`Headers` 组 `Authorization: Bearer` 和 `Content-Type: application/json`，`Endpoint` 拼 URL，`Encode` / `Decode` 用 `encoding/json` 改报文，`internal/server` 再用 `net/http` 的 `Client.Do` 发出去。`internal/llm` 不负责拨号，拨号在数据面。

协议分支只有四条加一个默认：

- 默认：`{api_base}/chat/completions`（以及 embeddings、completions、images、audio、moderations、rerank、responses、videos 的 OpenAI 路径）。未知但非空的供应商走这条。
- Azure：模型放进 `/openai/deployments/{model}/...`。
- Anthropic：聊天打 `/v1/messages`，缺 `max_tokens` 时补 256。
- Gemini 与 Vertex：`encodeGemini` 把文本消息收成 `generateContent` 的 `contents`。Vertex 的 URL 仍是占位路径 `/v1/projects/x/locations/us/publishers/google/models/{model}:generateContent`，项目和区域没有从部署参数拆开。

`internal/router.KnownAdapter` 对任意非空供应商返回 true。这只说明名字不是空字符串。它不代表该供应商的鉴权、请求体或响应已经按 LiteLLM 改过。`dataplane.go` 里的 `provider_not_implemented` 只出现在「没有尝试过上游 HTTP」时（没有可用部署，或凭证路径没有发出请求）。配置了 `api_base` 的未知供应商会被当成 OpenAI 兼容 HTTP 转发出去。这种转发不是语义对齐。

专用 `HandleFunc` 覆盖了密钥、用户、团队、组织、项目、预算、模型管理、一部分用量、健康检查、登录、邮件事件设置，以及聊天、补全、embeddings、messages、responses、音频翻译的主路径。这些函数读写 SQLite 或真的打上游。

其余目录路由先进 `catalogFallback`：`matchCatalog` 对照嵌入的 `routes.json`，命中之后管理面走 `writeCatalogPersist`（通用 KV），数据面在能认出的推理操作上转入 `dataPlane`，认不出的走 `writeInferenceNative` 或 `resourceCRUD`。能返回 JSON、状态不是 404，只说明兜底还在。兜底不是该家族的契约。

路由策略在 `internal/router.Pick`：`least-busy` 和 `lowest-cost` 各有分支，其它名字（含 `simple-shuffle`）都落到「权重最大，否则第一个」。catalog 里的 15 个策略名没有逐个实现。缓存是 `internal/cache` 的进程内 LRU（`hashicorp/golang-lru`）。预算和并行限制在 `internal/spend` 与 `internal/hooks`。持久化是 SQLite（`modernc.org/sqlite`），没有 Prisma，也没有 Redis。

## 2. 目标

HTTP 框架用 Gin。`ServeMux` 不是终态。`catalog.json` 里写的「net/http 或 gin」让位给这个决定：迁移完成后，对外监听的引擎是 Gin，方法、路径、状态码和响应体保持第 6 节各家族的契约。

实现保持 Go 的写法：小接口、按协议共享的编解码、显式的依赖，而不是把 Python 的一个类译成一个 Go 类型。`litellm/llms` 里上百个只改默认地址的子类，在 Go 里应合成「一份协议编解码 + 一张供应商差异表」。差异表要写明鉴权头、路径和报文字段。没有差异的供应商才允许共用一份编解码。

官方 Go SDK 必须复用，手写 HTTP 只留给没有维护中的 SDK 的协议：

| 协议 | 必须复用的模块 | 用途 |
|---|---|---|
| OpenAI Chat / Completions / Embeddings / Images / Audio / Responses / Files，以及只改 `api_base` 与 Bearer 密钥的兼容供应商 | `github.com/openai/openai-go` | 取代 `internal/llm` 里手写的 OpenAI JSON |
| Azure OpenAI（部署路径、`api-version`、`api-key`） | 同一个 `github.com/openai/openai-go` 的 Azure 选项 | 不再手拼 `/openai/deployments/{model}` |
| Gemini API 与 Vertex AI `generateContent` | `google.golang.org/genai` | 取代 `encodeGemini` / `decodeGemini` 和 Vertex 占位 URL |
| Anthropic Messages | `github.com/anthropics/anthropic-sdk-go` | Messages、count_tokens、工具块 |
| Bedrock 运行时与 Polly、S3 Vectors | `github.com/aws/aws-sdk-go-v2` 对应 service 客户端 | SigV4，禁止再套一层 OpenAI URL |

同一条规则适用于其余供应商：有维护中的官方 Go SDK 就用 SDK；没有就在该协议组里手写 HTTP，并给请求体、响应体单独测试。不得因为「看起来像 OpenAI」就跳过差异。

目标包按职责拆，路径不必对应 LiteLLM 的任何目录：

```text
cmd/gateway            进程入口。装配配置、存储和 Gin 引擎
internal/httpapi       Gin 路由、中间件、CORS、幂等键、错误体和 x-litellm-* 头
internal/auth          虚拟密钥、管理会话、JWT / OIDC、SSO
internal/store         SQLite。表按网关资源设计，不克隆 Prisma schema
internal/router        部署匹配与路由策略
internal/spend         用量、预算、限流计数
internal/cache         响应缓存。先保持进程内 LRU，后一阶段再排可替换后端
internal/hooks         调用前的预算与并行闸门
internal/provider      协议编解码。按协议分子目录，不按 Python 包名
internal/mgmt          管理面 handler。按资源分子目录（key、user、team…）
internal/config        YAML 配置
```

`internal/provider` 下面是协议，例如 `openai`、`anthropic`、`gemini`、`azure`、`bedrock`、`cohere`、`search`、`media`。一个协议服务多个供应商。供应商特有的头、路径和字段放在该协议的差异表里，不各建一个包。

## 3. 状态词

第 6 节每个家族用下面三个词之一。词描述的是**现在**的代码，不是计划做完之后。

- `aligned`：控制台或 SDK 客户端消费的方法与路径有专用 `HandleFunc`，响应来自 SQLite 或真实上游编解码，并且 `go test` 打到这些函数（`internal/server` 或 `internal/llm`）。`catalogFallback` 不算证据。
- `partial`：消费路径里只有一部分是专用 handler，或者整条路径只经过 `catalogFallback`（`dataPlane`、`resourceCRUD`、`writeCatalogPersist`、`writeInferenceNative`）。能返回 JSON，但不是该家族在 LiteLLM 里的完整契约。OpenAI 兼容转发也算在这里，不算 `aligned`。
- `missing`：没有该契约。长连接被收成普通 JSON、应原样转发的前缀被收成 KV、报表和工具路由没有专用形状，都记 `missing`。仅仅非 404 不能记成 `aligned`。

「把契约做实」的阶段号写在每个家族后面。阶段 0 到阶段 6 按顺序做。前一阶段的 `go test ./internal/server/ ./internal/llm/` 保持通过，才进入下一阶段。handler 若从 `internal/server` 挪到 `internal/httpapi` 或 `internal/mgmt`，测试跟着挪，仍然调用真正注册进 Gin 的处理函数，不另写一套 Python 对照器。

## 4. 阶段

### 阶段 0：换成 Gin，契约不动

把 `http.NewServeMux` 和 `http.ListenAndServe` 换成 Gin 引擎。现有专用 handler 逐条挂到相同的方法和路径上。`/` 上的 catalog 兜底先留着，并在代码注释外的本文档保持「兜底不是对齐」。本阶段不引入 OpenAI 或 Gemini SDK，避免换框架和换协议叠在一起。

证明：现有 `internal/server` 的 `httptest`（密钥、身份、数据面、邮件事件、家族往返）改为打 Gin 引擎之后仍然通过；`internal/llm` 的 `Endpoint` / `Encode` / `Decode` 测试原样通过。

本阶段把已经做实的家族迁上去，不改它们的 JSON：`mgmt.keys`、`mgmt.users`、`mgmt.teams`、`mgmt.projects`、`mgmt.email`。

### 阶段 1：OpenAI、Azure OpenAI、Anthropic、Gemini、Vertex 走官方 SDK

删除手写的 OpenAI JSON 和 `encodeGemini`。Vertex 不再使用 `projects/x/locations/us`。聊天、文本补全、embeddings、Messages、`/v1beta/models/{model}:generateContent` 改为专用 Gin 路由（含现在只在兜底里识别的 `engines`、`deployments`、`cursor`、`queue` 别名）。Messages 的 `count_tokens` 打上游，不再走 `writeInferenceNative`。

证明：`internal/llm`（或迁出后的 `internal/provider`）比较 SDK 请求的 URL、头和正文；`internal/server` 的数据面测试用 `httptest` 打聊天、补全、embeddings、messages、Gemini 路径。

### 阶段 2：其余已经能进 `dataPlane` 的推理路由改为专用注册

responses、images（generations 与 edits）、audio（speech、transcriptions、translations）、moderations、rerank、videos。今天除 responses 和 audio translations 之外，这些操作没有专用 `HandleFunc`，只是兜底认出 `inferenceOp` 后再调 `dataPlane`。阶段 2 为 catalog 里该家族的每条路径注册 Gin 路由。rerank 若供应商不是 OpenAI 形状，留到阶段 5 的协议组，本阶段至少让 OpenAI 兼容与 Cohere 形状分开，禁止一律 POST 到 `{base}/rerank`。

证明：`internal/server` 对每条路径发请求，断言不再依赖「先落到 `/` 再碰运气」。

### 阶段 3：补齐已有专用 handler 的管理契约

组织的 `/v2/organization/{organization_id}`、预算的 `/provider/budgets`、模型管理里控制台还在读的字段与 `/health/test_connection` 的真实探测、护栏列表、用量日志的字段、配置 YAML 的读与更新、SSO 回调与 `/fallback/login`、SCIM 的写和发现文档、健康检查的 `/settings`、回调列表、缓存设置里尚未专用的子路径、路由设置的更新、客户与最终用户的创建、邀请、UI 设置的更新与 logo、`/user/available_users` 以外的 `mgmt.enterprise_misc` 路径、模型列表与 model hub。访问组从通用 KV 收成专用 handler。

路由策略补到 catalog 的 15 个名字：`simple_shuffle`、`least_busy`、`lowest_cost` 已经有部分行为，其余 `adaptive_router`、`auto_router`、`budget_limiter`、`complexity_router`、`lar1_routing`、`lowest_latency`、`lowest_tpm_rpm`、`lowest_tpm_rpm_v2`、`quality_router`、`savings_baseline`、`tag_based_routing`、`base_routing_strategy` 要在 `internal/router` 里有各自的选择函数。未识别的策略返回明确错误，不再悄悄用权重最大的部署。

证明：`internal/server` 按家族补表驱动测试，请求打到 Gin 上的专用函数。`internal/router` 的测试直接调用 `Pick`。

### 阶段 4：把 KV 占位收成客户端消费的资源形状

文件、batch、assistants/threads、fine-tuning、containers、向量库数据面、skills/tools/memory、evals、workflows、agents、interactions、MCP 数据面，以及管理面上的 policies、prompts、credentials、tags、MCP server、search tools、向量库管理、JWT 映射、public 发现、允许 IP、告警设置、onboarding。今天这些大多是 `resourceCRUD` 或 `writeCatalogPersist`：字段名能让页面不崩，但没有文件内容、batch 状态机、assistant run、凭证与调用的绑定（除了模型参数上已有的 `Hydrate`）、邀请令牌校验。

本阶段的完成标准是请求和响应形状，加上 SQLite 里真实的状态变化。需要打到第三方的那一部分（例如真正创建上游 batch）跟阶段 5 的协议组一起做，不在占位 JSON 上宣布完成。

证明：`internal/server` 测试创建、读取、更新、删除，并断言存储里的行，而不是只断言 HTTP 200。

### 阶段 5：按第 7 节的协议组接上 136 个供应商包

每个包只属于一个协议组。组内用一份编解码。LiteLLM 改过鉴权、请求体或响应的供应商，差异写进表里，并用测试锁住。禁止用「非空名字 + `{base}/chat/completions`」当作该包已完成。`KnownAdapter == true` 在本阶段删除或改成「该协议组的差异表里有这个名字」。

搜索、OCR、RAG、A2A 的数据面家族在这一阶段和对应协议组一起做实，不再返回空的 `results` 或空的 `artifacts`。

证明：`internal/llm` / `internal/provider` 对每个协议组至少一条请求体与一条响应体测试；对每个「只改 base URL」的供应商一条 URL 与鉴权头测试；对每个有差异的供应商一条差异测试。数据面测试仍从 HTTP handler 进入。

### 阶段 6：长连接、原样转发，以及尚未有形状的管理契约

Realtime（含 `/vertex_ai/live`）改为 WebSocket 或供应商要求的长连接，不再返回 `{object, data:[]}`。`data.passthrough` 按前缀把原始路径和正文转到对应供应商，不把 `/bedrock/...` 收成 KV。Claude Code 的 marketplace 与插件清单返回客户端要的 JSON。合规报表、CloudZero 与 Vantage 导出、调试与内存、重载日程、审计日志、`/utils/token_counter` 与 `/utils/transform_request`、SCIM placeholders，各自有专用 handler。

审计、项目等 catalog 标了 `enterprise: true` 的 HTTP 契约仍要做。做法是按已观察到的请求和响应实现，不把 `enterprise/` 里的 Python 拷进仓库。

缓存后端（Redis）、可选的 Postgres、指标抓取放在本阶段排期，不在换 Gin 时一起上。SQLite 继续是默认存储。

证明：`internal/server` 用 `httptest` 覆盖握手前的 HTTP 升级拒绝/接受、passthrough 的出站 URL，以及各管理路由的 JSON 形状。

### 阶段 7：去掉「非 404 即完成」

779 条路由都由 Gin 的明确注册或带参数的路由覆盖。`catalogFallback` 删除。删掉之后，未注册的路径返回 404。再跑一遍 `internal/server` 里按 `routes.json` 行走的测试，断言每条路由命中的是专用处理函数或该家族的参数路由，而不是兜底。

本阶段不新增能力，只确认前面各阶段没有把契约留在兜底里。

## 5. 家族

下面 70 个 id 都来自 `catalog.json` 的 `families`。状态是现在的代码。阶段是把该家族消费契约做实的那一步。

### 数据面

| 家族 | 状态 | 阶段 | 现在的代码 |
|---|---|---|---|
| `data.chat` | partial | 阶段 1 | 专用 `POST /v1/chat/completions`、`POST /chat/completions` 进入 `dataPlane`。`engines`、`cursor`、`queue`、Azure deployment 别名只在兜底里被认成 chat。上游是手写 HTTP。Vertex 为占位路径。 |
| `data.completions` | partial | 阶段 1 | 专用 `POST /v1/completions`、`POST /completions`。`engines` 与 deployment 别名走兜底。 |
| `data.messages` | partial | 阶段 1 | 专用 `POST /v1/messages`。`/v1/messages/count_tokens` 被 `inferenceOp` 标成 `count_tokens` 后进入 `writeInferenceNative`，不打上游。 |
| `data.responses` | partial | 阶段 2 | 专用 `POST /v1/responses`、`POST /responses`。`Encode` 只把文本块收成 `input_text`，其余原样转发。`/openai/v1/responses` 走兜底。 |
| `data.embeddings` | partial | 阶段 1 | 专用 `POST /v1/embeddings`、`POST /embeddings`。Azure 有单独路径。其余供应商拼 `{base}/embeddings`。 |
| `data.images` | partial | 阶段 2 | 没有专用 `HandleFunc`。生成与编辑只因兜底认出 `images` / `images_edits` 才进入 `dataPlane`。 |
| `data.audio` | partial | 阶段 2 | 只有翻译有专用路由。speech 与 transcriptions 走兜底再进 `dataPlane`。speech 的 `Decode` 原样返回字节。 |
| `data.moderations` | partial | 阶段 2 | 无专用注册。兜底转入 `dataPlane`，URL 为 `{base}/moderations`。 |
| `data.rerank` | partial | 阶段 2 | 无专用注册。一律 `{base}/rerank`，没有 Cohere / Jina 的请求体。 |
| `data.files` | partial | 阶段 4 | 兜底进 `resourceCRUD`，本地 KV 填一个 `file` 对象，不调用上游 Files。 |
| `data.batches` | partial | 阶段 4 | 同上，本地 `batch` 停在 `validating`，没有批处理执行。 |
| `data.assistants_threads` | partial | 阶段 4 | 本地 assistant / thread 对象，没有 run。 |
| `data.fine_tuning` | partial | 阶段 4 | 本地 job，状态固定 `queued`。 |
| `data.containers` | partial | 阶段 4 | 本地 container，状态固定 `running`。 |
| `data.vector_stores` | partial | 阶段 4 | 数据面路径进本地 KV。没有上游向量库调用。 |
| `data.videos` | partial | 阶段 2 | 无专用注册。兜底把 `videos` 送进 `dataPlane`。 |
| `data.realtime` | missing | 阶段 6 | `inferenceOp` 认出 realtime 后走 `writeInferenceNative` 的默认 JSON，不是 WebSocket。 |
| `data.search_ocr_rag` | partial | 阶段 5 | `resourceCRUD` 返回空的 `results`。不是供应商搜索或 OCR。 |
| `data.skills_tools_memory` | partial | 阶段 4 | 混合路径进通用 KV 或 `resourceCRUD`。没有技能执行。 |
| `data.evals` | partial | 阶段 4 | 本地 eval 对象，状态 `created`。 |
| `data.workflows` | partial | 阶段 4 | KV。列表形状里有 `runs`，没有运行引擎。 |
| `data.agents` | partial | 阶段 4 | KV 里的 agent 记录。 |
| `data.access_groups` | partial | 阶段 3 | 不在数据面前缀上，落到管理兜底 KV。控制台要的列表没有专用 handler。 |
| `data.mcp` | partial | 阶段 4 | 混合兜底。带 `jsonrpc` 的请求返回空的 `tools`。OAuth token 是占位字符串。 |
| `data.a2a` | partial | 阶段 5 | 本地对象，`artifacts` 为空，状态直接 `completed`。 |
| `data.gemini_v1beta` | partial | 阶段 1 | 无专用注册。`/v1beta/models/...:generateContent` 经兜底进入 `dataPlane` 的 gemini 分支，编解码是手写的，且只保留文本。 |
| `data.passthrough` | missing | 阶段 6 | 没有按 `/openai`、`/anthropic`、`/bedrock`、`/gemini`、`/azure`、`/vertex_ai` 原样转发的处理。这些前缀要么撞上别的推理识别，要么变成管理 KV。 |
| `data.model_hub` | partial | 阶段 3 | `GET /v1/models` 与 `GET /models` 是专用 `listModels`。`/model_hub`、`/public/model_hub`、deprecations 走公共兜底或空列表。 |
| `data.interactions` | partial | 阶段 4 | 本地 interaction，缺省状态 `completed`，输出由输入文本拼出来。 |
| `data.claude_code` | missing | 阶段 6 | marketplace、插件和事件批量接口没有专用文档形状。 |

### 管理面

| 家族 | 状态 | 阶段 | 现在的代码 |
|---|---|---|---|
| `mgmt.keys` | aligned | 阶段 0 | `key/generate`、`list`、`info`、`update`、`delete`、`block`、`unblock`、`regenerate`、`bulk_update`、`aliases`、`health`、service-account 都是专用 handler，密钥表在 SQLite。`keys_test.go` 打这些入口。 |
| `mgmt.users` | aligned | 阶段 0 | `user/new`、`list`、`info`、`update`、`delete`、`/v2/user/info` 为专用 handler，用户行在 SQLite。 |
| `mgmt.teams` | aligned | 阶段 0 | `team/new`、`list`、`info`、`update`、`delete`、成员增删改、`/v2/team/list` 为专用 handler。 |
| `mgmt.organizations` | partial | 阶段 3 | `organization/new`、`list`、`info`、`update`、`delete` 与成员路由是专用 handler。`/v2/organization/{organization_id}` 没有专用注册。 |
| `mgmt.projects` | aligned | 阶段 0 | catalog 列出的 `project/new`、`update`、`delete`、`info`、`list` 都有专用 handler（含同路径的 GET）。这是 HTTP 契约，不是把 enterprise Python 搬过来。 |
| `mgmt.budgets` | partial | 阶段 3 | `budget/new`、`list`、`info`、`update`、`delete`、`/budgets`、`/budget/settings` 为专用 handler。`/provider/budgets` 没有专用注册。 |
| `mgmt.models` | partial | 阶段 3 | `model/new`、`update`、`delete`、`info`、`/v1/model/info`、`/v2/model/info` 以及 block 为专用 handler，改的是配置里的模型列表。不是完整的 model registry / capabilities。`/health/test_connection` 是简化探测。 |
| `mgmt.guardrails` | partial | 阶段 3 | 只有 `POST /apply_guardrail` 与 `POST /guardrails/apply_guardrail` 为专用 handler。`GET /guardrails` 与 `/v2/guardrails/list` 走兜底。 |
| `mgmt.policies` | partial | 阶段 4 | 无专用 handler。兜底 KV。 |
| `mgmt.prompts` | partial | 阶段 4 | 无专用 handler。兜底 KV，`freezeFamily` 补了 `version`。 |
| `mgmt.credentials` | partial | 阶段 4 | 无 `/credentials` 专用路由。模型调用前的 `llm.Hydrate` 会读已存的命名凭证，HTTP 的增删改仍是兜底 KV（响应里去掉 `credential_values`）。 |
| `mgmt.tags` | partial | 阶段 4 | `/tag/new`、`list`、`info`、`update`、`delete` 无专用 handler。`/spend/tags` 是用量聚合，不是标签管理。 |
| `mgmt.spend` | partial | 阶段 3 | `spend/logs`、`global/spend`、`global/activity` 及按 key、model、provider、team、tag 的聚合有专用 handler。字段和日活口径不是 LiteLLM 的全量日志。`/usage/ai/chat` 无专用注册。 |
| `mgmt.config` | partial | 阶段 3 | `GET /config/list` 与删除回调为专用 handler。`/config/yaml`、`/config/update`、Vault 覆盖、`/reload/model_cost_map` 走兜底。 |
| `mgmt.sso` | partial | 阶段 3 | `POST /login`、`/v2/login`、`/v3/login`、`/v3/login/exchange`、`GET /sso/key/generate` 为专用 handler。`/sso/callback` 与 `/fallback/login` 不是。 |
| `mgmt.scim` | partial | 阶段 3 | `GET /Users`、`GET /Groups` 从用户表和团队表投影成 SCIM 列表。写操作以及 `ResourceTypes`、`Schemas`、`ServiceProviderConfig` 无专用 handler。 |
| `mgmt.mcp_servers` | partial | 阶段 4 | `/server`、`/toolset`、`/tools` 无专用 handler。 |
| `mgmt.search_tools` | partial | 阶段 4 | `/search_tools` 与 `/search_tools/list` 走兜底。 |
| `mgmt.vector_stores_admin` | partial | 阶段 4 | `/vector_store/new`、`list`、`delete`、`info`、`update` 无专用 handler，与数据面共用 KV 形状。 |
| `mgmt.health` | partial | 阶段 3 | `liveliness`、`liveness`、`readiness`、`readiness/details`、`/health`、`/test`、`/health/services` 为专用 handler，readiness 会 ping SQLite。`/settings` 无专用注册。`test_connection` 不是对每个供应商的完整探测。 |
| `mgmt.callbacks` | partial | 阶段 3 | `GET /get/config/callbacks` 与 `POST /config/callback/delete` 为专用 handler。`/callbacks/list`、`/callbacks/configs`、`/logs` 不是。 |
| `mgmt.cache` | partial | 阶段 3 | `GET/POST /cache/settings`、`POST /flushall`、`GET /ping`、`GET /cache/ping` 为专用 handler，背后是进程内 LRU。`/redis/info`、`/delete`、coordination redis 无专用实现。 |
| `mgmt.router` | partial | 阶段 3 | `GET /router/settings` 与 `GET /router/fields` 读配置。`GET /auto_router/benchmarks` 返回静态结构。设置更新、`/auto_router/session`、`/adaptive_router/state`、`/fallback`、`/routes` 不是完整契约。 |
| `mgmt.jwt_oidc` | partial | 阶段 4 | `/jwt/key/mapping/list` 与 `/new` 无专用 handler，也没有在鉴权链里生效的映射。 |
| `mgmt.customers` | partial | 阶段 3 | `GET /customer/list` 与 `GET /end_user/list` 为专用 handler，读的是 KV。`POST /customer/new` 与 `/end_user/new` 不是专用路由。 |
| `mgmt.invitations` | partial | 阶段 3 | `/invitation/new`、`info`、`update`、`delete` 无专用 handler。 |
| `mgmt.compliance` | missing | 阶段 6 | 没有 EU AI Act 或 GDPR 报表 handler。 |
| `mgmt.cost_export` | missing | 阶段 6 | 没有 CloudZero 或 Vantage 导出。 |
| `mgmt.ui_settings` | partial | 阶段 3 | `GET /get/ui_settings` 与 `GET /get/ui_theme_settings` 走 `uiSettings`。`GET /get_logo_url` 是 `emptyOK`。`/update/ui_settings` 与 `/upload/logo` 无专用写契约。 |
| `mgmt.public` | partial | 阶段 4 | `GET /public/...` 在兜底里返回嵌入的字段表、空 hub 或空列表，不是可更新的发现服务。 |
| `mgmt.debug` | missing | 阶段 6 | 没有内存摘要、OTel span 或 lazy warm 的专用 handler。 |
| `mgmt.ops_schedules` | missing | 阶段 6 | 没有 model cost map 或 Anthropic beta 头的重载日程。 |
| `mgmt.audit` | missing | 阶段 6 | 没有 `/audit` 列表或单条读取。实现时按 HTTP 契约写，不移植 enterprise Python。 |
| `mgmt.email` | aligned | 阶段 0 | `GET/PATCH /email/event_settings` 与 `POST /email/event_settings/reset` 为专用 handler，`email_events_test.go` 覆盖。 |
| `mgmt.enterprise_misc` | partial | 阶段 3 | `GET /user/available_users` 为专用 handler。`/log-event` 与 `/robots.txt` 不是。 |
| `mgmt.utils` | missing | 阶段 6 | `/utils/token_counter`、`/utils/transform_request`、`/utils/supported_openai_params` 无专用 handler。数据面内部的 `estimateTokens` 不是这三条路由。 |
| `mgmt.allowed_ips` | partial | 阶段 4 | 获取、添加、删除允许 IP 走兜底 KV。请求链上没有 IP 允许名单中间件。 |
| `mgmt.alerting` | partial | 阶段 4 | `/alerting/settings` 走兜底。`catalogListBody` 对这个路径返回列表，没有告警发送。 |
| `mgmt.placeholders` | missing | 阶段 6 | `/placeholders` 没有 SCIM placeholder 文档形状。 |
| `mgmt.onboarding` | partial | 阶段 4 | `/onboarding/get_token` 与 `/onboarding/claim_token` 无专用 handler。 |

## 6. 供应商：136 个包按协议组覆盖

分母是 `catalog.json` 的 `providers` 数组，共 136 个名字。磁盘上 `litellm/llms` 还有 `base.py`、`custom_llm.py`、`maritalk.py`，它们不在这 136 个里。`custom_llm` 的用户自定义回调放在 `internal/hooks` 的扩展点，不单开一个 Python 式的包。`maritalk.py` 不在分母内，不实现。

分组依据是该包在 LiteLLM 1.102.0 里的配置基类（`OpenAIGPTConfig`、`OpenAILikeChatConfig`、`AnthropicMessagesConfig`、`BaseSearchConfig` 等），不是 `KnownAdapter`。今天除 Azure、Anthropic、Gemini、Vertex 四个分支外，其余非空名字都走 OpenAI URL。那一行为在迁移完成前全部保持 `partial` 或 `missing`，不能因为名字出现在下面的「字节兼容」组里就记成已经对齐。

字节兼容组的完成标准：出站 JSON 与 OpenAI 该操作相同，差异只有 `api_base` 和 `Authorization: Bearer`。若实现时发现 LiteLLM 还改了头、路径或字段，该包改记到「OpenAI 差异」组，并补差异测试。用转发冒充差异，不算完成。

### 协议组 `openai_byte_compatible`（35）

使用 `github.com/openai/openai-go`，只替换 Base URL 与 Bearer 密钥。

`ai21`、`aiohttp_openai`、`amazon_nova`、`baseten`、`cerebras`、`clarifai`、`cloudflare`、`codestral`、`compactifai`、`datarobot`、`docker_model_runner`、`empower`、`featherless_ai`、`friendliai`、`galadriel`、`gdc`、`github`、`gradient_ai`、`heroku`、`hyperbolic`、`inception`、`lambda_ai`、`lemonade`、`llamafile`、`lm_studio`、`meta_llama`、`moonshot`、`morph`、`nebius`、`novita`、`nscale`、`oobabooga`、`v0`、`wandb`、`zai`

### 协议组 `openai_native`（1）

`openai`。Chat、Completions、Embeddings、Images、Audio、Responses、Files、Batches、Fine-tuning、Realtime、Videos、Containers、Evals、Vector Stores 都走 `github.com/openai/openai-go`，按操作选择 SDK 方法，不手写 URL 表。

### 协议组 `openai_delta`（31）

仍然用 `github.com/openai/openai-go` 作公共报文，但 LiteLLM 另外改了鉴权、第二协议或非聊天操作。每一条差异要有测试。只转发到 `/chat/completions` 不算这组完成。

`aiml`、`chatgpt`、`cometapi`、`databricks`、`deepinfra`、`deepseek`、`fireworks_ai`、`github_copilot`、`groq`、`hosted_vllm`、`huggingface`、`litellm_proxy`、`manus`、`minimax`、`mistral`、`modelscope`、`nvidia_nim`、`openai_like`、`openrouter`、`ovhcloud`、`perplexity`、`sambanova`、`sap`、`snowflake`、`tencent`、`together_ai`、`vercel_ai_gateway`、`vllm`、`volcengine`、`watsonx`、`xai`

要点写进差异表，不新开 Python 式目录。本组里同时带 Messages 形状的有 deepseek、minimax、tencent、github_copilot、databricks、openai_like。chatgpt 与 github_copilot 的鉴权不是静态 Bearer。hosted_vllm 的图像编辑、视频、转写在 LiteLLM 里继承 OpenAI 的对应配置，要走 SDK 的对应方法，而不是聊天 URL。perplexity 另有搜索。watsonx 用 IBM 的混入改嵌入、重排和透传。xai 的 realtime 放到阶段 6。

### 协议组 `azure_openai`（1）

`azure`。使用 `github.com/openai/openai-go` 的 Azure 选项：部署路径、`api-version`、`api-key` 头。responses 的默认版本按 LiteLLM 的 preview 路径，不复用公有云 `{base}/responses`。

### 协议组 `azure_ai_foundry`（1）

`azure_ai`。子协议能对上 SDK 的用 SDK（OpenAI 兼容聊天、Anthropic Messages），OCR、agents、rerank、图像用该子协议自己的 HTTP。不把整个包转发到一个 `/chat/completions`。

### 协议组 `anthropic_messages`（1）

`anthropic`。使用 `github.com/anthropics/anthropic-sdk-go`。覆盖 messages、count_tokens、files、batches、skills。实验性透传归 `passthrough` 行为（阶段 6），不把透传前缀当成 Messages。

### 协议组 `gemini_genai`（2）

`gemini`、`vertex_ai`。使用 `google.golang.org/genai`。Vertex 用项目、区域和 ADC（或部署里的 `vertex_credentials`），删除占位位置 `projects/x/locations/us`。图像、视频、文件、realtime、interactions 使用该 SDK 的对应 API；SDK 没有的操作再手写，并单独测试。

### 协议组 `bedrock_aws`（5）

`bedrock`、`bedrock_mantle`、`sagemaker`、`aws_polly`、`s3_vectors`。使用 `github.com/aws/aws-sdk-go-v2`。bedrock_mantle 的正文接近 OpenAI，但签名是 AWS，禁止只换 Base URL。Polly 走 Polly 客户端。S3 Vectors 走 S3 Vectors / S3 客户端。

### 协议组 `cohere_http`（1）

`cohere`。聊天、嵌入、rerank、OCR 分操作实现。若 `github.com/cohere-ai/cohere-go` 仍维护，则用它；否则在本组手写 HTTP。不得用 `{base}/chat/completions`。

### 协议组 `search_http`（17）

无需要复用的官方 Go SDK 时手写 HTTP。每个包保留自己的查询参数和结果字段。

`apiserpent`、`brave`、`dataforseo`、`duckduckgo`、`exa_ai`、`fastcrw`、`firecrawl`、`google_pse`、`linkup`、`nimble`、`parallel_ai`、`searchapi`、`searxng`、`serper`、`tavily`、`tinyfish`、`you_com`

### 协议组 `image_media_http`（7）

`black_forest_labs`、`fal_ai`、`recraft`、`runwayml`、`stability`、`topaz`、`xinference`。图像、视频或变体各自的创建与轮询。有官方 Go SDK 的用 SDK，没有的手写。

### 协议组 `audio_http`（5）

`deepgram`、`elevenlabs`、`nvidia_riva`、`scaleway`、`soniox`。转写或语音合成。有官方 SDK 的用 SDK。

### 协议组 `embed_rerank_http`（4）

`dashscope`、`infinity`、`jina_ai`、`voyage`。dashscope 的聊天类是 DashScopeChatConfig，不是 OpenAI 子类。infinity 的 rerank 继承 Cohere 形状，嵌入仍是它自己的。

### 协议组 `vector_store`（5）

`milvus`、`mongodb`、`pg_vector`、`ragflow`、`valkey`。按各存储的查询 API 实现。pg_vector 在 LiteLLM 里继承 OpenAI vector store 配置，差异表要写明哪些字段可沿用 OpenAI SDK。

### 协议组 `sandbox`（2）

`e2b`、`opensandbox`。沙箱执行，不是聊天补全。

### 协议组 `ocr_http`（1）

`reducto`。OCR 请求与结果字段单独编解码。

### 协议组 `own_protocol_http`（13）

这些包的聊天或主操作不是 OpenAI 子类。手写该协议，或使用该厂商仍在维护的 Go SDK。

`a2a`、`bytez`、`gigachat`、`langflow`、`langgraph`、`meta`、`nlp_cloud`、`oci`、`ollama`、`petals`、`predibase`、`replicate`、`triton`

meta 是 realtime，与阶段 6 一起做。ollama 使用它自己的 /api/chat，或它声明的兼容端点，以 LiteLLM 的 BaseConfig 变换为准，不默认改成 /chat/completions。

### 协议组 `shared_runtime`（4）

这四项不是对外供应商名字，但在 136 个包里，必须有归宿，不能丢掉：

- `base_llm`：变成 `internal/provider` 的接口（补全、嵌入、图像、rerank、透传、向量），不移植 Python 抽象类文件。
- `custom_httpx`：传输层。Go 用 `net/http` 或 SDK 自带的客户端，不移植 httpx 传输。
- `pass_through`：护栏翻译钩子，归 `internal/hooks`，不是一条模型 URL。
- `deprecated_providers`：明确拒绝（例如已删除的 Palm、Aleph Alpha），返回与 LiteLLM 相同的错误语义，禁止转发到 OpenAI。

### 计数

35 + 1 + 31 + 1 + 1 + 1 + 2 + 5 + 1 + 17 + 7 + 5 + 4 + 5 + 2 + 1 + 13 + 4 = 136。

## 7. 测试

每一阶段都用仓库里已有的测试方式，不新造外部对照服务：

- `go test ./internal/server/`：`httptest` 调用进程的 `Handler()`（阶段 0 之后是 Gin 引擎），断言状态码和 JSON 字段。现有的 `keys_test.go`、`dataplane_test.go`、`family_test.go`、`email_events_test.go`、`identity_runtime_test.go` 是模板。
- `go test ./internal/llm/`：比较 `Endpoint`、`Encode`、`Decode` 的字节。迁到 `internal/provider` 之后，测试仍调用对外导出的编解码函数，不断言私有辅助函数的拷贝。

禁止把期望 JSON 写死在与实现无关的地方再让测试自己组一份相同的报文。测试要调用即将随进程发布的函数。

## 8. 不在这次后端迁移里做的事

1. 不移植 `enterprise/` 下非 MIT 许可的 Python 源码。catalog 里标了 enterprise 的 HTTP 路径仍按公开观察到的契约实现。
2. 不克隆 LiteLLM 的 Python 包目录，也不克隆 Prisma schema。存储继续用现有 SQLite 模型，按 Go 结构体扩展。
3. 不在这次后端迁移里重写 Next.js 控制台。`frontend/` 保持不动。控制台已经在调用的路径，由上面的管理面阶段把响应形状做实。
4. 不把 OpenAI 兼容转发当成语义对齐。只要 LiteLLM 改了该供应商的鉴权、请求体或响应，该包就必须走差异表或它自己的协议组。`KnownAdapter` 返回 true、或者未知供应商打到 `{api_base}/chat/completions`，都不是完成。

另外，本计划本身不切换运行时代码。换 Gin、加 SDK、补 779 条路由，是按第 4 节落地的后续工作。
