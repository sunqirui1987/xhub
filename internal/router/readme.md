# router

Orders the deployments that share one public model name. It does not send HTTP and it does not read Redis itself. The caller passes a `State` snapshot.

## Identity

`DeploymentID` is `api_base|model`. Both pieces come from `ModelEntry.ParamString`. An empty `model` parameter falls back to the public `ModelName`. Both empty produces the string `"|"`. Redis cooldown, latency, route TPM, and spend use this id (`xhub:cooldown:`, `xhub:latency:`, `xhub:routetpm:`, `xhub:spend:`). Key RPM/TPM do not. Those use `Principal.Hash`.

## Order of a choice

`matchDeployments` keeps rows whose public name or wildcard pattern matches `alias`. `Order` asks `Pick` for the first usable row, then appends the rest of the pool so a failure can try the next one.

`Pick` drops deployments whose id is in `State.Cooldown` when at least one other deployment is still open. If every candidate is cooling, the cooling ones stay and are attempted anyway.

`strategyKind` maps the config name. Hyphens become underscores first. Unknown names return ok false. `ValidateStrategy` turns that into the error `unknown routing strategy`. `Serve` writes that as HTTP 400. It does not substitute `simple-shuffle`.

| Config name | Internal kind | Who wins |
| --- | --- | --- |
| empty, `simple-shuffle`, `simple_shuffle`, `base_routing_strategy`, `adaptive_router`, `auto_router`, `complexity_router`, `quality_router` | `weight` | highest weight |
| `least-busy`, `least_busy` | `busy` | lowest `State.Busy` |
| `lowest-cost`, `budget_limiter`, `savings_baseline` | `cost` | lowest cost parameter |
| `lowest-latency`, `lar1_routing`, `latency_based_routing` | `latency` | lowest `State.Latency` |

`State` zero value has no cooldown and no latency. The gateway fills it from `dataplane.State` when Redis is configured, and always fills `Busy` from the in-process map.

`AdapterURL` and `AdapterURLOp` forward to `llm.Endpoint`. The path table for `/chat/completions`, `/v1/messages`, and the rest lives there, not here.

`EncodeRequest` / `DecodeResponse` in `adapter.go` are the old names for `llm.Encode` and `llm.Decode`.

## What this package does not do

It does not pin a session. Chat pins (`deployment_affinity:v1:`, one hour) and official task pins (`official_task:v1:`, seven days) are applied by the data plane after `Order`, via `preferDeployment`. It does not skip a paused deployment; `serve.go` `dropPaused` does that and returns 400 `model_paused` when nothing remains.

中文说明见同目录 `readme_cn.md`。
