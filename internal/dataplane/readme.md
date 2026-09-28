# dataplane

## Purpose

`dataplane` sends one inference call. It picks an ordered list of deployments, encodes the body, calls the upstream, and moves to the next deployment when the current one fails. It also records cooldown, latency, and spend deltas in Redis. It does not import `gateway`, so the HTTP process and the send loop cannot form an import cycle.

## Features

- `Serve` is the entry a gateway handler calls for chat, embeddings, images, and the other inference operations.
- `EstimateTokens` gives budget and TPM checks an upper bound before the call.
- `State` builds the router snapshot (cooldown, latency, usage, in-flight counts) from the host.
- `RecordFailure`, `RecordLatency`, and `RecordUsage` write hot-path facts after a call.
- `Flush` and `FlushLoop` move queued spend from Redis into PostgreSQL.

## How the gateway uses it

The gateway implements `dataplane.Host`. A handler does not call providers itself:

```go
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
    dataplane.Serve(s, w, r, "chat")
}
```

A host must supply config, the store, the HTTP client, the router state, credentials, guardrails, the plugin registry, and the spend recorder. See `Host` in `host.go`. Extensions registered on that host run before the cache and before the upstream HTTP call. A refusal skips the provider.

`FlushLoop` is started from `Server.Run` only when Redis is configured. Calling `Flush` yourself applies the currently queued deltas once.

## What this package does not do

It does not parse the master key and it does not register URL patterns. Authentication happens before `Serve`.

中文使用说明见同目录的 readme_cn.md。
