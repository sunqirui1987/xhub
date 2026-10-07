# dataplane

`dataplane` sends one model call. It does not import `gateway`. The process implements `Host` in `gateway/wire.go` and calls `Serve` or `ServeBypass`.

`Host` embeds three interfaces. A caller is only required to implement the one it uses. `internal/dataplane/capability_test.go` locks the signatures:

| Interface | Entry | What it may call |
| --- | --- | --- |
| `Adapted` | `Serve` in `serve.go` | chat, embedding, image, audio, rerank. No official task pin, no Redis flush |
| `Bypass` | `ServeBypass` in `official.go` | official forwarder. No response cache, no guardrails, no chat encoding |
| `Runtime` | `State` and `Flush` in `live.go` | Redis snapshot and spend flush. No HTTP |

## Adapted loop (`Serve`)

`gateway/limits.go` `dataPlane` calls `Serve(h, w, r, op)` for ordinary inference. Family handlers for images, audio, rerank, videos, responses, files, and realtime end in the same function.

Order inside `Serve`:

1. `RequireLLMPrincipal`. A nil principal means the response is already 401 and `Serve` returns.
2. Read the body. Invalid body or invalid JSON is 400 `invalid_request`. A missing `model` field is 400 `model required`.
3. `EstimateTokens`, then `EnforceIdentityLimits`. A refusal is already written (budget or rate limit). `Serve` does not write a second body.
4. For `op == "chat"` or empty op, `GuardrailBlocks`. A block records the exchange, calls `RecordSpend` with success false and status 400, then writes 400 `guardrail_failed`. Other operations skip guardrails.
5. `HookEngine().Begin(p.KeyID)` only counts the in-flight call. It does not check budget. See `internal/hooks`.
6. `applyExtensions`. The first plugin that sets `Decision.Refuse` stops the call. Headers already set by earlier plugins stay.
7. Non-stream only: `ResponseCache.Get`. The key is `cache.Key(tenant, op, alias, rawBody)`. Tenant is `Principal.Hash` when the caller has a key, otherwise empty. A hit calls `WriteCacheHit` and returns. The dollar amount of a cache hit is 0; token counts are kept. Stream responses are not read from this cache.
8. `router.ValidateStrategy`. An unknown strategy is 400, not silent `simple-shuffle`.
9. `router.Order` then `preferDeployment` moves the chat pin (`PlanRoute`) to the front. Paused deployments are dropped. An empty pool is 400 `model_paused` when every candidate was paused, otherwise 400 `model not found: <alias>`.
10. The outer loop walks every deployment in the pool. `NumRetries` does not cap that list. It is the inner HTTP attempt count for one deployment (`for try := 0; try < attempts`). A configured value below 1 is raised to 1. `ResolveRequest` and `EnforceIdentityLimits` run again only when the deployment index `di > 0`, that is when moving to a later deployment, not on each 5xx or 429 of the same deployment. Each inner attempt calls `IncBusy` and then POSTs. Status >= 500 or 429 calls `DecBusy`, then `NoteFailure`, and continues the inner loop. A dial error (`HTTPClient.Do` returns an error) calls `DecBusy` and continues the inner loop without `NoteFailure`. Building the request (`NewRequestWithContext` fails) does the same: `DecBusy`, continue, no `NoteFailure`. When the inner loop is finished, the outer loop tries the next deployment.
11. Non-stream success: `WriteChatJSON` writes the body and records spend. Stream success is copied by `stream.go`, which writes the bytes itself and then `RecordSpend`. `AnnotateCall` runs before that write. Non-stream chat sets `TTFTMs` from the whole elapsed time. Stream sets it from the first byte.

`usage.go` fills token counts the upstream omitted. Tool-call argument text is not added into output tokens. `log.go` redacts bearer tokens, `sk-` keys, and full upstream URLs before a log line. `secretInErr` lives only in `log.go`.

## Bypass loop (`ServeBypass`)

`gateway/bypass.go` matches `provider.Match` (kind bypass) before Gin and calls `ServeBypass`. The body is not rewritten into chat completions.

Create (`serveBypassCreate`):

- `pickDeployment` selects one deployment for this model. No match is 400 `model not found: <alias>`. A missing model field is 400 `<field> required`. Create does not 400 because several `api_base|api_key` groups exist.
- The public model field (`hit.Type.ModelField`, `model` for both Seedance types) is replaced with `OfficialID`, which strips one provider prefix (`volcengine` or `qiniu`).
- `withQuery` appends `r.URL.RawQuery` to the upstream URL when the query is non-empty. It does not read `RequestURI`. `r.URL.RequestURI()` is the `url` field of the proxy log document built later by `gateway/spend.go` `promptJSON`, not the messages document stored as the prompt.
- Create calls `RecordSpend` with a nil usage map, so this response is not billed from usage. The response task id is pinned with `PinOfficial` for 7 days under `official_task:v1:<taskID>`.

Follow (`serveBypassFollow`):

- A list (empty task id) calls `oneUpstream`. No eligible model is 400 `model not found`. More than one upstream `api_base|api_key` group is 400 `model required: <names>`.
- A get loads the pin with `OfficialDeployment`. A missing pin is 404.
- `sameEndpoint` returns false for a Volcengine task id on a Qiniu path and the reverse. `serveBypassFollow` then writes HTTP 404 `unknown task`. The two contents APIs are not interchangeable.
- The first follow-up whose body contains usage is billed once. `OfficialBilled` reads `official_billed:v1:<taskID>`. `MarkOfficialBilled` is set before `RecordSpend`. A later poll with usage is not billed again.

`AnnotateCall` on this path uses `dataplaneNote` (provider prefix, session, deployment, elapsed). There is no guardrail pass and no response cache.

## Runtime (`State`, `Flush`)

`State` asks `Redis()` for cooldown, latency, and route TPM, and `BusyMap()` for in-process concurrency. Redis nil means `State` only fills `Busy`. `Flush` writes queued spend into `Identity()`. A nil identity returns immediately and persists nothing. `Flush` does not speak HTTP.

## What this package does not do

It does not listen, does not own sessions, and does not implement the Gin catalog. Those are `internal/gateway`. It does not register endpoint types. Those are `internal/provider`.

中文说明见同目录 `readme_cn.md`。
