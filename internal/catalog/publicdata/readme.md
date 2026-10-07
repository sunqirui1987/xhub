# catalog/publicdata

Static JSON embedded into the gateway binary by `internal/catalog`. The process does not read these files from disk at startup. Tests and `./cmd/pricedata` are the writers.

| File | What it is |
| --- | --- |
| `pricedata.json` | Model price rows generated from the Modelink market feed `https://api.modelink.ai/v1/market/models`. Every key is a real model id. There is no LiteLLM sample row and no `sample_spec`. |
| `routes.json` | The HTTP catalog the gateway mounts after its own modules. `catalog.Load` returns these rows. A path that is not in this file and not on a module is a JSON 404. |
| Provider and field JSON next to those | The public add-model payload: endpoint types are registered in Go (`internal/provider`), while this directory holds the generated price and route documents. |

`catalog.loadPriceDocument` parses `pricedata.json` once. A parse failure leaves an empty map so the process can still start; `Cost` then returns ok false and the caller must not record the call as free. `LITELLM_LOCAL_MODEL_COST_MAP=true` forces the built-in map and skips a remote price source.

Hand-entered prices do not rewrite this file. `catalog/price_write.go` keeps a baseline snapshot and lays database overrides on top. Clearing an override restores the baseline row.

中文说明见同目录 `readme_cn.md`。
