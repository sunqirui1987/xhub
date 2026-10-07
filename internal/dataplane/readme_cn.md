# dataplane

`dataplane` 发出一次模型调用。它不导入 `gateway`。进程在 `gateway/wire.go` 里实现 `Host`，再调用 `Serve` 或 `ServeBypass`。

`Host` 嵌了三个接口。调用方只实现自己用到的那一个。`internal/dataplane/capability_test.go` 锁住了签名：

| 接口 | 入口 | 能调用的能力 |
| --- | --- | --- |
| `Adapted` | `serve.go` 的 `Serve` | 聊天、向量、图像、音频、重排。没有官方任务钉，没有 Redis 刷写 |
| `Bypass` | `official.go` 的 `ServeBypass` | 官方接口转发。没有响应缓存，没有护栏，没有聊天编码 |
| `Runtime` | `live.go` 的 `State` 和 `Flush` | Redis 快照和花费刷写。不处理 HTTP |

## 适配循环（`Serve`）

普通推理由 `gateway/limits.go` 的 `dataPlane` 调用 `Serve(h, w, r, op)`。图像、音频、重排、视频、responses、文件和 realtime 的 family 处理函数最后也进同一个函数。

`Serve` 内部顺序：

1. `RequireLLMPrincipal`。主体是 nil 表示响应已经是 401，`Serve` 直接返回。
2. 读正文。坏的正文或坏的 JSON 是 400 `invalid_request`。没有 `model` 字段是 400 `model required`。
3. `EstimateTokens`，然后 `EnforceIdentityLimits`。拒绝时响应已经写好（预算或限流）。`Serve` 不再写第二份正文。
4. `op` 是 `chat` 或空时走 `GuardrailBlocks`。拦住时先记下交换，再以失败和状态 400 调用 `RecordSpend`，然后写 400 `guardrail_failed`。其他操作不跑护栏。
5. `HookEngine().Begin(p.KeyID)` 只计在途次数，不查预算。见 `internal/hooks`。
6. `applyExtensions`。第一个把 `Decision.Refuse` 设上的插件停掉这次调用。前面插件已经写上的头保留。
7. 只有非流式才 `ResponseCache.Get`。键是 `cache.Key(租户, op, 对外名, 原始正文)`。调用方有密钥时租户是 `Principal.Hash`，否则为空。命中则 `WriteCacheHit` 并返回。缓存命中的金额是 0，token 数保留。流式响应不读这份缓存。
8. `router.ValidateStrategy`。不认识的策略是 400，不会静默当成 `simple-shuffle`。
9. `router.Order` 之后 `preferDeployment` 把聊天钉（`PlanRoute`）移到最前。暂停的部署被丢掉。池子空了：全是暂停时 400 `model_paused`，否则 400 `model not found: <对外名>`。
10. 外层循环走完池子里的每一条部署。`NumRetries` 不限制部署条数。它是同一条部署上的内层 HTTP 次数（`for try := 0; try < attempts`）。配置值小于 1 时提升到 1。只有部署下标 `di > 0`、也就是换到后面的部署时，才再次 `ResolveRequest` 和 `EnforceIdentityLimits`。同一次部署上的 5xx 或 429 不会重新解析身份。每一次内层尝试先 `IncBusy` 再 POST。状态 >= 500 或 429 先 `DecBusy`，再 `NoteFailure`，然后继续内层循环。拨号失败（`HTTPClient.Do` 返回 error）只 `DecBusy` 并继续内层循环，不调用 `NoteFailure`。构造请求失败（`NewRequestWithContext` 返回 error）也一样：`DecBusy`、继续、不调用 `NoteFailure`。内层用完后，外层才试下一条部署。
11. 非流式成功由 `WriteChatJSON` 写正文并记用量。流式成功由 `stream.go` 自己抄字节，然后 `RecordSpend`。写用量之前先 `AnnotateCall`。非流式聊天把整段耗时写成 `TTFTMs`。流式用第一个字节的时间。

`usage.go` 补上上游没给的 token 数。工具调用的参数文本不加进输出 token。`log.go` 在写日志前打掉 bearer、`sk-` 密钥和完整上游 URL。`secretInErr` 只在 `log.go`。

## 官方转发（`ServeBypass`）

`gateway/bypass.go` 在 Gin 之前做 `provider.Match`（种类是 bypass），然后调用 `ServeBypass`。正文不会被改写成聊天补全。

创建（`serveBypassCreate`）：

- `pickDeployment` 为这个模型选一条部署。没有命中是 400 `model not found: <对外名>`。模型字段缺失是 400 `<字段名> required`。创建不会因为存在多组 `api_base|api_key` 而返回 400。
- 公开模型字段（两种 Seedance 都是 `model`，字段名在 `hit.Type.ModelField`）换成 `OfficialID`。`OfficialID` 只去掉一层供应商标前缀（`volcengine` 或 `qiniu`）。
- `withQuery` 在查询串非空时把 `r.URL.RawQuery` 接到上游 URL 后面。它不读 `RequestURI`。`r.URL.RequestURI()` 是稍后 `gateway/spend.go` 的 `promptJSON` 写进代理日志文档 `url` 字段的路径，不是当作提示词存下的 messages 文档。
- 创建调用 `RecordSpend` 时 usage 为 nil，所以这次响应不按 usage 扣费。响应里的任务 id 用 `PinOfficial` 钉 7 天，键是 `official_task:v1:<任务id>`。

后续查询（`serveBypassFollow`）：

- 列表（任务 id 为空）调用 `oneUpstream`。没有可用模型是 400 `model not found`。多于一组上游 `api_base|api_key` 是 400 `model required: <名字列表>`。
- 查询用 `OfficialDeployment` 取出钉。没有钉是 404。
- `sameEndpoint` 对火山任务 id 走七牛路径（以及反过来）返回 false。`serveBypassFollow` 接着写 HTTP 404 `unknown task`。这两套内容生成接口不能互换。
- 第一次在正文里看到 usage 的后续请求记一次账。`OfficialBilled` 读 `official_billed:v1:<任务id>`。先 `MarkOfficialBilled`，再 `RecordSpend`。之后再带 usage 的查询不再记账。

这条路径的 `AnnotateCall` 用 `dataplaneNote`（供应商前缀、会话、部署、耗时）。没有护栏，也没有响应缓存。

## 运行时（`State`、`Flush`）

`State` 向 `Redis()` 要冷却、延迟和路由 TPM，向 `BusyMap()` 要进程内并发。Redis 是 nil 时 `State` 只填 `Busy`。`Flush` 把排队的花费写进 `Identity()`。身份库是 nil 时立刻返回，什么都不落盘。`Flush` 不写 HTTP。

## 这个包不做什么

它不监听端口，不拥有会话，也不实现 Gin 目录。那些在 `internal/gateway`。它不登记端点类型。那些在 `internal/provider`。

English notes are in `readme.md` in this directory.
