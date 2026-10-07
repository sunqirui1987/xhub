# hooks

`hooks` counts how many calls one virtual key has in flight inside this gateway process. It does not decide whether the call is allowed.

## Entry

`gateway/server.go` calls `hooks.New()` while assembling the process and stores the engine on `Server.Hooks`. The data plane never constructs its own engine. `Serve` reaches the shared one through `Host.HookEngine`.

`New` always returns a non-nil `*Engine`. The only state is `inflight map[string]int` behind a mutex. The map key is `Principal.KeyID`, the account id of the virtual key, not `Principal.Hash` and not the deployment id `api_base|model`.

## What Begin does

`dataplane.Serve` calls `Begin` only after `EnforceIdentityLimits` has already accepted the request:

```text
done := h.HookEngine().Begin(p.KeyID)
defer done()
```

- An empty `keyID` returns a no-op function and does not touch the map. A session with no key still gets a release function.
- Any other id increments `inflight[keyID]` and returns a function that decrements it. When the count falls to 0 or below, the id is deleted.

`Begin` returns one function. It does not return a reason string, an error, or a boolean. It never looks at `max_budget`, RPM, TPM, or Redis. There is no `"budget"` result and no `"parallel"` result. A full map does not refuse the call; this package does not even expose a way to read the count.

The deferred release runs when `Serve` returns, including the paths that later fail at the upstream. `ServeBypass` does not call `Begin`. An official task create or poll does not take a slot here.

## What this package does not do

Budget for the key, the team, the project, and the organization is checked earlier, in `gateway/limits.go` (`EnforceIdentityLimits`) using rows from `internal/iam`. RPM and TPM against Redis are also there: `enforceRedisRateLimits` calls `live.Client.HitRPM` and `HitTPM` with `Principal.Hash`, the token hash, on keys `xhub:rpm:` and `xhub:tpm:`.

The in-flight count is per process. A second gateway has its own map. Spend, cooldown, and the spend queue live in `internal/live`.

中文说明见同目录 `readme_cn.md`。
