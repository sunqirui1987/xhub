# router

给同一个对外模型名下的多条部署排序。它不发 HTTP，自己也不读 Redis。调用方传入一份 `State` 快照。

## 身份

`DeploymentID` 是 `api_base|model`。两段都来自 `ModelEntry.ParamString`。`model` 参数为空时退回对外的 `ModelName`。两段都空时得到字符串 `"|"`。Redis 的冷却、延迟、路由 TPM 和花费用这个 id（`xhub:cooldown:`、`xhub:latency:`、`xhub:routetpm:`、`xhub:spend:`）。密钥的 RPM/TPM 不用它，那些用 `Principal.Hash`。

## 一次选择的顺序

`matchDeployments` 留下对外名或通配模式对上 `alias` 的行。`Order` 先用 `Pick` 取出第一条还能用的，再把池子里其余的接在后面，所以上游失败时还能试下一条。

`Pick` 在 `State.Cooldown` 里的部署，只要还有别的部署开着，就不会排进去。如果每条都在冷却，冷却中的那些会留下来，照样去试。

`strategyKind` 映射配置里的名字。先把连字符换成下划线。不认识的名字返回 ok 为 false。`ValidateStrategy` 把这种情况变成错误 `unknown routing strategy`。`Serve` 把它写成 HTTP 400，不会改成 `simple-shuffle`。

| 配置名 | 内部种类 | 谁先被选 |
| --- | --- | --- |
| 空、`simple-shuffle`、`simple_shuffle`、`base_routing_strategy`、`adaptive_router`、`auto_router`、`complexity_router`、`quality_router` | `weight` | 权重最高 |
| `least-busy`、`least_busy` | `busy` | `State.Busy` 最低 |
| `lowest-cost`、`budget_limiter`、`savings_baseline` | `cost` | 成本参数最低 |
| `lowest-latency`、`lar1_routing`、`latency_based_routing` | `latency` | `State.Latency` 最低 |

`State` 的零值没有冷却也没有延迟。配了 Redis 时网关用 `dataplane.State` 填它，`Busy` 则始终来自进程内的表。

`AdapterURL` 和 `AdapterURLOp` 转到 `llm.Endpoint`。`/chat/completions`、`/v1/messages` 这些路径表在那边，不在这里。

`adapter.go` 里的 `EncodeRequest` / `DecodeResponse` 是 `llm.Encode` 和 `llm.Decode` 的旧名字。

## 这个包不做什么

它不钉会话。聊天钉（`deployment_affinity:v1:`，一小时）和官方任务钉（`official_task:v1:`，七天）是数据面在 `Order` 之后用 `preferDeployment` 贴上去的。它也不丢掉暂停的部署；`serve.go` 的 `dropPaused` 做这件事，池子空了就返回 400 `model_paused`。

English notes are in `readme.md` in this directory.
