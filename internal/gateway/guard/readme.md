# gateway/guard

## Purpose

`guard` runs content rules before a request is sent upstream. A blocking rule stops the data plane from contacting the provider. The dashboard can also trial a rule without spending provider quota.

## HTTP and Go entry points

- `PreCall` inspects a JSON body. It returns true and a message when the call must stop.
- `Apply` is the HTTP trial endpoint mounted by `Module`.
- The process calls `GuardrailBlocks` on the server, which delegates here, from the data plane before the upstream HTTP client is used.

## How a caller uses the result

```go
if blocked, message := guard.PreCall(host, body); blocked {
    httpx.WriteTypedError(w, r.URL.Path, 400, "guardrail_violation", message)
    return
}
```

Rules are the guardrail rows stored for the proxy. A rule marked default-on applies even when the request does not name it. A request that names extra guardrails adds those rules.

## What this package does not do

It does not call the model to classify text unless a stored rule says so. The default matcher is the word list on the guardrail row.

中文使用说明见同目录的 readme_cn.md。
