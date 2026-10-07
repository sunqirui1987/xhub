# catalog/publicdata

Static JSON embedded into the gateway binary by `internal/catalog`. The process does not read these files from disk at startup. Tests and `./cmd/pricedata` are the writers.

This directory contains two JSON files: `pricedata.json` and `autorouter_presets.json`. It does not contain `routes.json`, and it does not contain provider or field JSON.

| File | What it is |
| --- | --- |
| `pricedata.json` | Model price rows generated from the Modelink market feed `https://api.modelink.ai/v1/market/models`. Every key is a real model id. There is no LiteLLM sample row and no `sample_spec`. `catalog/embed.go` embeds it with `//go:embed publicdata/pricedata.json`. `loadPriceDocument` parses those bytes once at startup. |
| `autorouter_presets.json` | Preset document for a public GET whose path contains `/public/autorouter_presets`. Embedded by `//go:embed publicdata/autorouter_presets.json` in the same `embed.go`. `PublicBody` unmarshals it. A parse failure returns `{"presets": []}`. |

`routes.json` is `internal/catalog/routes.json`, embedded by `//go:embed routes.json` in `catalog/embed.go`. `catalog.Load` returns those rows. A path that matches neither a gateway module, nor that catalog, nor a bypass endpoint is the Gin `NoRoute` JSON 404.

`catalog.loadPriceDocument` parses `pricedata.json` once. A parse failure leaves an empty map so the process can still start; `Cost` then returns ok false and the caller must not record the call as free.

`LITELLM_LOCAL_MODEL_COST_MAP=true` (case ignored, surrounding space trimmed) makes `catalog.EnvForced` return true. `gateway/models.CostMapSource` copies that bool into `is_env_forced` on GET `/model/cost_map/source`. `gateway/catalog.go` also wraps `EnvForced` as `localCostMapForced`, and nothing calls that wrapper. The flag does not replace the live map with the embedded file, and it does not skip a remote fetch. `models.ReloadCostMap` and `models.scheduledReloadLoop` do not read it. Both call `reloadNow`, and `reloadNow` always calls `catalog.ReloadFromMarket`.

Hand-entered prices do not rewrite this file. `catalog/price_write.go` keeps a baseline snapshot and lays database overrides on top. Clearing an override restores the baseline row.

中文说明见同目录 `readme_cn.md`。
