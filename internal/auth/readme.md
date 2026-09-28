# auth

## Purpose

`auth` answers one question for every HTTP request: who is calling, and what are they allowed to do. The gateway does not parse `Authorization` inside each handler. Login, virtual-key calls, and the master key all become a `Principal` here.

## Features

- Reads the bearer token from the request.
- Recognizes the master key from configuration.
- Resolves a virtual key through the store and attaches the stored key record.
- Distinguishes management rights from inference rights.
- Reports authentication failures with `IsAuthErr` so handlers can return 401 without guessing the error type.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/auth`.

```go
principal, err := auth.Resolve(cfg, st, r)
if err != nil {
    if auth.IsAuthErr(err) {
        httpx.WriteError(w, 401, "auth_error", err.Error())
        return
    }
    httpx.WriteError(w, 500, "internal", err.Error())
    return
}
if !principal.CanManage() {
    httpx.WriteError(w, 403, "forbidden", "management key required")
    return
}
if principal.CanLLM(cfg) {
    // safe to enter the inference data plane
}
```

`APIKeyFrom` is the lower-level helper when you only need the raw token. `CanManage` is for dashboard and admin routes. `CanLLM` is for chat, embeddings, and the other inference routes. A virtual key fails `CanLLM` when the configuration disallows it or the key itself cannot call models.

## What this package does not do

It does not create keys, store passwords, or choose a model deployment. Those belong to `gateway/keys`, `store`, and `router`.

中文使用说明见同目录的 readme_cn.md。
