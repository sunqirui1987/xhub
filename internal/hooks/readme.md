# hooks

## Purpose

`hooks` stops a call before it spends money. It checks the key budget and the number of calls already in flight for that key. When either limit is already exhausted, the data plane must not contact the upstream.

## Features

- `New` creates an empty gate. Counts are kept in memory for this process.
- `Begin` reserves one slot. It returns a release function and an empty reason on success.
- When the key budget is already spent, `Begin` returns the reason `"budget"` and a nil release function.
- When the parallel limit is full, `Begin` returns `"parallel"` and a nil release function.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/hooks`.

```go
gate := hooks.New()
release, reason := gate.Begin(key)
if reason != "" {
    // "budget" or "parallel": do not call the provider
    return
}
defer release()
// contact the upstream
```

The gateway stores one engine on `Server.Hooks` and exposes it with `HookEngine`. Use that instance so every request shares the same in-flight map. A nil key is allowed through and the release function is a no-op.

## What this package does not do

It does not read Redis and it does not write spend. Hot spend and cooldown live in `live`.

中文使用说明见同目录的 readme_cn.md。
