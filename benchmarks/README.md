# xhub 基准测试与压力测试

从项目根目录运行，使用项目已有 Go 依赖，无需额外安装压测工具。默认启动真实网关、真实 PostgreSQL 和本地 OpenAI 协议上游，每次创建独立 schema；正常结束、阈值失败或 SIGINT/SIGTERM 后删除本次数据。默认不调用付费供应商，也不读取生产网关配置。

## 快速开始

需要 Go（版本要求见根目录 go.mod）及可连接的 PostgreSQL，数据库账号必须有创建/删除 schema 权限。默认使用开发数据库 127.0.0.1:5433，其他数据库通过 XHUB_TEST_DATABASE_URL 指定 PostgreSQL URL。数据库不可达时直接失败，不把跳过测试当作成功。

    go run ./benchmarks -concurrency 1,4 -requests 20 -warmup 2

默认完整阶梯是并发 1、4、16、64，各跑四场景，每阶段最多 1000 请求或发压 15 秒。请求上限和时间上限先到者停止发新请求，在途请求继续直到完成或超时；总运行时间还包含初始化、预热、在途排空和清理。

## 压力配置

并发阶梯，观察吞吐拐点与尾延迟：

    go run ./benchmarks -concurrency 1,8,32,64,128 -requests 1000000 -duration 10s -warmup 20 -out benchmarks/reports/staircase

慢上游模拟，观察连接占用和首字延迟：

    go run ./benchmarks -scenarios chat,stream -concurrency 1,16,64 -requests 1000000 -duration 10s -upstream-delay 200ms -out benchmarks/reports/slow-upstream

大请求正文（正文之外还包含少量唯一标记）：

    go run ./benchmarks -scenarios chat -concurrency 16,64 -prompt-bytes 65536 -requests 1000000 -duration 10s -out benchmarks/reports/large-prompt

持续负载，以下只跑一个场景、一个并发级别，持续最多十分钟：

    go run ./benchmarks -scenarios stream -concurrency 64 -requests 1000000 -duration 10m -warmup 20 -out benchmarks/reports/soak

CI 阈值，错误率必须为零且 P95 不超过 500ms，否则退出码为 1，仍保存报告并清理：

    go run ./benchmarks -scenarios chat,stream -concurrency 1,16 -requests 100 -max-error-rate 0 -max-p95-ms 500

## 场景与指标

| 场景 | 实际链路与成功条件 |
| --- | --- |
| chat | 个人密钥鉴权 → 路由 → 普通推理 → 非空回答 → 真实用量持久化 |
| stream | 个人密钥鉴权 → 路由 → SSE → 非空 content 与完整 [DONE] → 用量持久化 |
| models | GET /v1/models，返回测试模型 |
| auth-reject | 随机无效密钥必须返回 HTTP 401 和 error；预期拒绝算场景成功 |

每阶段报告请求/成功/失败数、错误分类、HTTP 状态码、RPS、成功 RPS、P50/P95/P99/最大延迟、响应字节量，以及流式首字时间 TTFT。流内错误、截断 SSE、错误 JSON 和只有 HTTP 200 但无业务结果都计入失败。无重试、不跟随重定向，避免掩盖真实错误或把密钥发往重定向目标。

延迟分位数使用 nearest-rank，包含失败请求；TTFT 仅统计完整成功的流式请求。RPS 分母包含在途排空时间。每阶段预热独立发请求，只预热服务状态，不保留连接池；预热不计入阶段吞吐，但计入总用量审计。

隔离模式结束时核对上游调用数、成功用量条数、每次固定 11 输入/5 输出 token 和每次 0.000062 USD 的账面费用。此费用用于验证平台计费，不产生供应商账单。审计失败也返回非零退出码。

## 已有网关

先在终端环境中设置 XHUB_BENCHMARK_KEY 为测试专用个人密钥，然后指定实际网关和实际模型：

    go run ./benchmarks -base-url http://127.0.0.1:4000 -model YOUR_MODEL -concurrency 1,4,16 -requests 100 -out benchmarks/reports/existing

该模式会产生目标网关的真实推理费用、用量和请求日志；工具不创建/删除目标网关实体，也不清除这些日志。应使用专用测试网关和测试密钥。上游延迟参数只对默认隔离模式生效，已有网关模式没有隔离数据审计。

## 报告与清理

报告默认写入 benchmarks/reports/<UTC时间>-<PID>/report.json 和 report.md；可用 -out 指定目录。隔离网关的逐请求诊断保存在同目录 diagnostic.log，终端只输出阶段摘要；压测仍包含网关日志写入成本。统计报告不含密钥、数据库 URL 或请求正文，reports/ 已忽略 Git。使用同一输出目录会覆盖之前的报告，应给每次保留的运行指定不同目录。

按 Ctrl+C 会停止发压并保存已有统计、执行清理，退出码为 1。SIGKILL/主机掉电无法执行清理；报告里的 Schema 字段用于识别本次私有 schema，人工核实后可删除该确切 schema，禁止批量删除其他运行的数据。报告 CleanupOK 表示隔离资源清理是否成功，在已有网关模式下为 false（不适用）。

## 验证工具自身

单元测试（包含并发竞争检测，无数据库依赖）：

    go test -race ./benchmarks ./benchmarks/load ./benchmarks/fixture -run 'Test(Parse|Run|Summarize|Validate|Read|Request|Open|JSON)' -count=1

真实后台 regression（数据库不可达时失败，核对四场景、费用损坏检测、删除 schema）：

    go test ./benchmarks ./benchmarks/fixture -run 'Test.*Regression$' -count=1 -v

浏览器 E2E（使用现有隔离 E2E 运行器；需要项目前端依赖、Playwright 浏览器与 Docker PostgreSQL）：

    bash scripts/e2e.sh benchmark.spec.ts

E2E 从页面创建个人密钥，运行八个 CLI 压测阶段，核对真实用量与 token，在页面打开对应日志，页面删除密钥后验证推理返回 401；finally 清理测试密钥，运行器删除私有 schema。

## 如何解释容量

这是固定并发闭环负载：一个工作者完成请求后再发下一次。服务变慢时发压速率自然下降，因此无法模拟固定到达速率下不断增长的队列（coordinated omission）；不能把这里的 P95 当作生产高峰 SLO 保证。应在吞吐不再提升而 P95 明显上升时，选更低并发继续长时间复测。

默认客户端、网关和本地上游共用一台主机，包含真实鉴权、路由、日志和 PostgreSQL 持久化，但没有 Redis、多实例、真实供应商网络、护栏、媒体生成或业务特定配置。当前实现不收集进程 CPU/RSS 或数据库连接指标；定位瓶颈需结合平台和数据库监控。生产容量评估应部署独立发压机，并通过已有网关模式测试与生产相同的拓扑。

本地上游只返回短回答且固定 token，无法代表长输出生成性能。隔离网关上游超时为 30 秒；模拟延迟接近/超过此值会触发可观察失败。客户端单请求超时可用 -timeout 调整，HTTP 响应上限为 4MiB，SSE 单行上限为 1MiB。
