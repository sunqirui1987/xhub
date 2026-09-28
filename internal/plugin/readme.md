# plugin

## Purpose

`plugin` is the pre-call extension point. An extension sees the operation, model, and path, and it may change response headers or refuse the call. A refusal means the data plane does not read the cache and does not contact the upstream. This is not a loader for `.so` files. You register a Go value by name.

## Features

- `New` returns an empty registry. `Run` on an empty registry allows the call.
- `Register` appends an extension. A duplicate name fails and the old order stays.
- `Run` calls every extension in order. The first refusal stops the rest and keeps headers already set.
- `Invoke` runs one extension by the name it registered.
- `Names` returns a copy of the names in order.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/plugin`.

```go
type refuseChat struct{}

func (refuseChat) Name() string { return "gate-refuse" }
func (refuseChat) BeforeUpstream(c plugin.Call) plugin.Decision {
    if c.Op == "chat" && c.Model == "blocked-model" {
        return plugin.Decision{Refuse: true, Status: 403, Code: "extension_refused", Message: "blocked"}
    }
    return plugin.Decision{}
}

reg := plugin.New()
if err := reg.Register(refuseChat{}); err != nil {
    log.Fatal(err)
}
decision := reg.Run(plugin.Call{Op: "chat", Model: "blocked-model", Path: "/v1/chat/completions"})
if decision.Refuse {
    httpx.WriteError(w, decision.Status, decision.Code, decision.Message)
    return
}
```

The gateway keeps one registry and exposes it with `Extensions`. Register extensions on that value before serving traffic. An empty registry leaves chat, spend, cooldown, and the skip of unknown providers unchanged.

## What this package does not do

It does not know about HTTP and it does not import the gateway. The data plane calls `Run` and obeys `Refuse`.

中文使用说明见同目录的 readme_cn.md。
