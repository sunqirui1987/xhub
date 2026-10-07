# gateway/models

Model list, add-model, block, and the in-memory price overrides. Mounted as `models.Module` before the catalog.

GET `/v1/models` and GET `/models` list deployments the caller may see. `AllowLLM` is false for the master key unless `allow_master_key_llm` is on, and a view that cannot call still may list metadata it was granted. GET `/model/available` is the card list for the signed-in caller. Calling a model stays on the inference routes.

POST `/model/new` writes one deployment into `proxy_models` and the in-process `ModelTable`. The body selects `endpoint_types` (multi-select). Empty falls back to `mode`, then to `chat`. A custom bypass is `litellm_params.endpoint`, not a new Go file. POST `/model/update` and PATCH `/model/{model_id}/update` edit that row. POST `/model/block` sets `model_info.blocked`. A blocked deployment stays in the table and is refused at request time. POST `/model/delete` removes the row.

`LockModels` / `UnlockModels` wrap those writes. `LoadStored` merges database rows over the YAML list at startup and drops provider shells that were saved as if they were models.

## Built-in providers

`SeedBuiltins` runs when `XHUB_BUILTIN_PROVIDERS` is not `off`. It does not insert models. It creates two credentials if they are missing:

| id | Catalog URL | Call base | Env |
| --- | --- | --- | --- |
| `fennoai` | `https://api.fenno.ai/v1/models` | `https://api.fenno.ai` | `FENNOAI_API_KEY` |
| `qiniu` | `https://api.qnaigc.com/v1/models` | `https://api.qnaigc.com/bypass/openai/v1` | `QINIU_API_KEY` |

That Qiniu base is the OpenAI-compatible bypass used when adding chat models from the Qiniu catalog. It is not the contents API. Contents generation is `provider/qiniu` at `https://api.qnaigc.com` path `/v3/contents/generations/tasks`.

`providerKey` reads a key only to fetch the catalog. A masked placeholder is ignored. The saved model stores the credential name, not a copy of the secret.

## Price routes

POST `/price/model` and DELETE `/price/model` change the in-memory override. POST `/price/model/reset` restores the baseline row from `catalog`. POST `/reload/model_cost_map` fetches the market feed. POST `/schedule/model_cost_map_reload` arms `scheduledReloadLoop`, which ticks every minute and calls `catalog.ReloadFromMarket` when `next_run` has passed. A failed fetch leaves the prices in use and does not push `next_run` forward.

## What this package does not do

It does not encode a chat body. After a deployment is chosen, `dataplane.Serve` or `ServeBypass` does that.

中文说明见同目录 `readme_cn.md`。
