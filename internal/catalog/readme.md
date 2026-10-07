# catalog

The embedded description of routes and prices. JSON lives in `publicdata/` and is compiled into the binary. `init` calls `loadPriceDocument` and records `modelCostMapLoadedAt`. A parse failure leaves an empty map so the process still starts.

## Routes

`Load` returns every row of `routes.json`. The gateway mounts its own modules first (`gateway/routes.go`), then walks this list in `ingress.go`. The first method and path wins. There is no `"/"` catch-all. A path that matches neither a module, nor this catalog, nor a bypass endpoint is the Gin `NoRoute` JSON 404.

`IsPublicPath` is a public GET that can be answered without a session. `PublicBody` is the fixed JSON for that GET. Price rows and create-field documents come from the embedded files. Anything else is an empty list.

`IsDataPlanePath` is true for mixed prefixes and for `IsLLMPrefix`. `IsLLMPrefix` compares the path with `HasPrefix` against `/v1/chat`, `/v1/messages`, `/api/v3/contents`, `/v3/contents`, and the other inference prefixes in `classify.go`. `HasPrefix` trims one trailing slash on each side, so `/v1/chat` does not match `/v1/chatcompletions`. `PathMatch` treats `{name}` as one segment and `:path` / `{x:path}` as the rest. `Split` splits on `/` and returns nil for an empty path, not a slice containing `""`.

`IsMixedPath` is the old shared key-value families (`/v1/agents`, `/v1/skills`, `/v1/workflows`, and the rest). `gateway/family` `ServeMixed` requires an identity and then refuses them. They are not served from one global namespace.

## Prices

`CostMap` is model id to field map. `Cost` multiplies prompt and completion tokens by `input_cost_per_token` and `output_cost_per_token`. An unknown model returns ok false. Callers must not store that call as zero dollars. `KnownProvider` is true when the prefix is a supplier slug in the map, not an organization id such as `meta-llama`. Wildcard expansion strips only a known provider prefix.

`ProviderModels` returns the ids indexed for one provider, or nil. `Count` is the number of object rows. `EnvForced` is true when `LITELLM_LOCAL_MODEL_COST_MAP` is `true`, ignoring case.

Deployment fields `input_cost_per_token` and `output_cost_per_token` override this catalog inside gateway spend. They do not rewrite `pricedata.json`. `price_write.go` snapshots the loaded document as the baseline. `SetModel` / `RemoveModel` / `SetProvider` / `RemoveProvider` change the in-memory map. `ApplyDocument` replaces the live map with a generated document and refuses an empty document so a failed fetch cannot wipe the prices in use. `adoptBaseline` then re-snapshots, so "restore built-in" restores the prices the process is serving.

`Contribute` inserts one model row and remembers an official id so a later bill for that id finds the same rates. Empty id or empty provider writes nothing.

## What this package does not do

It does not match a bypass request. `provider.Match` does that before Gin. It does not choose a deployment. `router.Order` does that.

中文说明见同目录 `readme_cn.md`。
