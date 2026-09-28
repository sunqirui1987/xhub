# catalog

## Purpose

`catalog` is the static description of the gateway surface: which URLs exist, which of them are public, which enter the inference data plane, and what the built-in model prices are. The JSON ships inside the binary. The process does not read a routes file from disk at startup.

## Features

- `Load` returns every route from the embedded `routes.json`.
- `AuthOf`, `IsPublicPath`, `IsDataPlanePath`, `IsMixedPath`, and `IsLLMPrefix` classify a URL.
- `PathMatch` compares a catalog template such as `/key/{key}` with a concrete path.
- `PublicBody` returns the fixed JSON for a public GET.
- `CostMap`, `Count`, `ProviderModels`, and `KnownProvider` read the embedded price map.
- `Cost` multiplies prompt and completion tokens by that row's per-token prices. `Format` prints the dollar amount without trailing zeros. A missing row returns `ok == false`.
- `MarkReloaded` records that the in-memory price map was refreshed.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/catalog`.

```go
for _, rt := range catalog.Load() {
    if catalog.IsDataPlanePath(rt.P) {
        // mount on the inference side
    }
}
if catalog.IsPublicPath(r.Method, r.URL.Path) {
    httpx.WriteJSON(w, 200, catalog.PublicBody(r.URL.Path))
    return
}
prices := catalog.CostMap()
info := prices["gpt-4o-mini"]
```

Unknown providers are not listed by `KnownProvider`. Do not treat a missing name as OpenAI-compatible. The data plane skips those deployments.

## What this package does not do

It does not register Gin routes and it does not send HTTP. Mounting is the gateway's job. The files under `publicdata` are the embedded documents this package reads.

中文使用说明见同目录的 readme_cn.md。
