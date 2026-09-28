# llm

## Purpose

`llm` turns one logical call into the HTTP request a provider expects, and turns the provider bytes back into the public JSON shape. It does not dial. It does not read PostgreSQL. The data plane sends the `Upstream` value this package builds.

## Features

- `Build` returns the URL, headers, and body for a `Request`. OpenAI-compatible providers use the OpenAI SDK shape. Gemini and Vertex use the Gemini shape. Azure uses the OpenAI JSON with an `api-key` header and a deployment URL.
- `Encode` and `Decode` are the smaller pair the router adapter calls. `Decode` puts the caller's model alias back into the response.
- `ProtocolGroup` in this package maps a provider name to its wire group. An unknown provider must be skipped by the caller.
- `Hydrate` copies credential-store values into deployment parameters. Missing fields stay as they were.
- `StripProxyParams` removes gateway-only fields before the body is sent, so a provider does not reject an unknown parameter.
- `ExceptionForStatus` maps an upstream HTTP status to a LiteLLM exception name. Statuses without a branch return `ok == false`.
- `Allow` and `Filter` decide which URL prefixes are legal inference mounts.
- `PassthroughURL` joins a base and an endpoint when the body is already in the provider's own protocol.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/llm`.

```go
group, ok := llm.ProtocolGroup(provider)
if !ok || group == "" {
    // skip this deployment; do not pretend it is OpenAI-compatible
    continue
}
up, err := llm.Build(ctx, llm.Request{
    Op: "chat", Provider: provider, APIBase: apiBase, APIKey: apiKey,
    Model: realModel, Body: publicBody,
})
if err != nil {
    return err
}
req, _ := http.NewRequest(http.MethodPost, up.URL, bytes.NewReader(up.Body))
req.Header = up.Header
```

`APIBase` and `APIKey` must already be filled by the credential layer. `Build` does not replace an empty base with the vendor's public host.

## What this package does not do

It does not pick which deployment wins and it does not record spend.

中文使用说明见同目录的 readme_cn.md。
