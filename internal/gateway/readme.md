# gateway

The process that listens for HTTP and owns `Server`. Subpackages mount routes as `httpx.Module` and do not import this package. `wire.go` is the one place `*Server` implements their `Host` interfaces, plus `dataplane.Host`.

Listeners are not in this package's constants. The console is a separate Next.js process on `:3000`. This process is the API, default `:4000`. `PublicOrigin` is `XHUB_PUBLIC_ORIGIN` with the trailing slash removed, or `http://localhost:4000`. The console is not mounted under that origin. Playground snippets and the console's own API client use the gateway origin, not `:3000`.

## Request order

`Handler` in `engine.go` is the entry tests mount. It does not mount Gin as the first match.

1. Idempotency key replay, when the same key, method, and path already completed with a status below 500.
2. `serveBypass` in `bypass.go`. `provider.Match` on method and path, kind bypass, runs before Gin. A hit calls `dataplane.ServeBypass` and never enters the chat encoder.
3. Gin. `installModules` registers, in order: health, session, keys, models, tokens, ingress, access, identity, usage, prefs, guard, family. Then ingress mounts every remaining `catalog.Load` route. The first method and path wins. A later registration is skipped.
4. A catalog path whose handler is the default `serveFamilyRoute` calls `dataPlane` in `limits.go`, which calls `dataplane.Serve`. Images, audio, rerank, videos, responses, files, and realtime have their own family handlers and still end in that data plane.
5. No `"/"` route. `newEngine` `NoRoute` writes JSON 404 `not_found`, not Gin's plain text.

`recordSpend` in `spend.go` writes the usage row for every one of those paths. `AnnotateCall` attaches provider, TTFT, session, and deployment before that write. Prompt storage is optional and keeps headers, body, and response on the same row. Create of an official task does not bill. The first follow-up whose body has usage bills once.

## Pins

Chat affinity (`affinity.go`) uses `deployment_affinity:v1:session:<alias>:<8-byte caller hash>:<sessionID>` for one hour (`affinityTTL`). `previous_response_id` looks up `deployment_affinity:v1:response:<id>` first. Session id order is a client session header, then a cache key, then the previous response, then a hash of the stable prompt prefix. `CommitRoute` writes the pin after a successful adapted call. Bypass does not use this pin to choose a deployment.

Official tasks use `official_task:v1:<taskID>` for seven days (`officialPinTTL`). `official_billed:v1:<taskID>` is the bill-once mark, same TTL. Both go through `live.SetString` when Redis is configured, and an in-process map otherwise.

## Spend and limits

`EnforceIdentityLimits` checks the model allow-list, budget, and rate limits before `Serve` contacts an upstream. Redis RPM/TPM call `HitRPM` / `HitTPM` with `Principal.Hash`, not `api_base|model`. In-flight counting is `hooks.Begin(keyID)` inside `Serve` only.

`persistSpend` writes PostgreSQL immediately when Redis is nil. Otherwise the row is queued on `xhub:spendlog` and `flushLoop` calls `dataplane.Flush` every 60 seconds.

## What this package does not do

It does not serve the console HTML. It does not register endpoint types; import `internal/provider/all` for that. It does not decide team roles; `internal/authz` does, and this package applies the decision.

中文说明见同目录 `readme_cn.md`。
