# httpx

## Purpose

`httpx` writes the JSON the gateway returns and stamps a call ID on the response. Handlers do not set `Content-Type` or assemble the error object themselves. Inference routes can use the provider-shaped error. Management routes use the gateway envelope.

## Features

- `CallID` makes a new identifier. `SetCallID` writes it onto the response.
- `WriteJSON` writes a status code and a JSON body.
- `WriteError` writes the gateway error envelope: `error.type` and `error.message`.
- `WriteTypedError` picks the envelope shape from the request path. Anthropic message paths and Gemini native paths get their own shape. Everything else gets the gateway envelope.
- `Bind` builds a `Module`. `Mount` registers `METHOD /path` handlers on a `Registrar`. Feature packages depend on this surface instead of importing the process.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/httpx`.

```go
httpx.SetCallID(w, httpx.CallID())
if err != nil {
    httpx.WriteError(w, 400, "invalid_request", err.Error())
    return
}
httpx.WriteJSON(w, 200, map[string]any{"status": "ok"})
```

On an inference failure, pass the request path so the client sees the shape it expects:

```go
httpx.WriteTypedError(w, r.URL.Path, 429, "rate_limit_error", "slow down")
```

## What this package does not do

It does not log the call and it does not record spend. The call ID is only a response header value the caller can correlate.

中文使用说明见同目录的 readme_cn.md。
