# gateway

监听 HTTP 并持有 `Server` 的进程。子包以 `httpx.Module` 挂路由，不导入这个包。`wire.go` 是 `*Server` 实现那些 `Host` 接口以及 `dataplane.Host` 的唯一地方。

监听端口不是这个包里的常量。控制台是另一个 Next.js 进程，在 `:3000`。本进程是 API，默认 `:4000`。`PublicOrigin` 是去掉末尾斜杠的 `XHUB_PUBLIC_ORIGIN`，否则 `http://localhost:4000`。控制台不挂在这个源下面。Playground 复制出来的示例和控制台自己的 API 客户端都用网关的源，不用 `:3000`。

## 请求顺序

测试挂上的入口是 `engine.go` 的 `Handler`。它不会让 Gin 做第一次匹配。

1. 幂等键回放。同一个键、方法和路径已经以低于 500 的状态完成过，就回放那次响应。
2. `bypass.go` 的 `serveBypass`。在 Gin 之前按方法和路径做 `provider.Match`，种类是 bypass。命中就调用 `dataplane.ServeBypass`，不再进入聊天编码器。
3. Gin。`installModules` 按顺序登记：health、session、keys、models、tokens、ingress、access、identity、usage、prefs、guard、family。然后 ingress 挂上 `catalog.Load` 里还没被占掉的路由。先登记的方法和路径赢。后面的登记会被跳过。
4. 目录路径的默认处理函数 `serveFamilyRoute` 调用 `limits.go` 的 `dataPlane`，再调用 `dataplane.Serve`。图像、音频、重排、视频、responses、文件和 realtime 有自己的 family 处理函数，最后仍进同一个数据面。
5. 没有 `"/"` 路由。`newEngine` 的 `NoRoute` 写 JSON 404 `not_found`，不是 Gin 的纯文本。

`spend.go` 的 `recordSpend` 给上面每条路径写用量行。写之前 `AnnotateCall` 附上供应商、TTFT、会话和部署。提示词存储是可选的，把头、正文和响应留在同一行。官方任务的创建不记账。第一次正文里带 usage 的后续查询记一次。

## 钉

聊天粘滞（`affinity.go`）用 `deployment_affinity:v1:session:<对外名>:<调用方哈希前 8 字节>:<会话id>`，有效期一小时（`affinityTTL`）。`previous_response_id` 先查 `deployment_affinity:v1:response:<id>`。会话 id 的顺序是客户端会话头、缓存键、上一次响应、稳定提示前缀的哈希。适配调用成功后 `CommitRoute` 写下这根钉。Bypass 不用这根钉来选部署。

官方任务用 `official_task:v1:<任务id>`，七天（`officialPinTTL`）。`official_billed:v1:<任务id>` 是只记一次账的标记，同样的有效期。配了 Redis 时两者都走 `live.SetString`，否则留在进程内的表。

## 花费和限额

`EnforceIdentityLimits` 在 `Serve` 联系上游之前检查模型允许名单、预算和速率。Redis 的 RPM/TPM 用 `Principal.Hash` 调用 `HitRPM` / `HitTPM`，不用 `api_base|model`。在途计数是 `Serve` 里面的 `hooks.Begin(keyID)`，只有适配循环会调用。

Redis 是 nil 时 `persistSpend` 在请求里直接写 PostgreSQL。否则行进 `xhub:spendlog`，`flushLoop` 每 60 秒调用 `dataplane.Flush`。

## 这个包不做什么

它不提供控制台 HTML。它不登记端点类型；那要导入 `internal/provider/all`。它不判定团队角色；那是 `internal/authz`，这个包只执行判定结果。

English notes are in `readme.md` in this directory.
