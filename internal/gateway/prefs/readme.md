# gateway/prefs

Router and general settings stored in PostgreSQL and laid over the YAML `Config`. Mounted as `prefs.Module`.

GET `/router/settings` and GET `/router/fields` render the settings page document: strategy, retries, timeout, and the field catalog (`generalFieldCatalog` includes prompt-cache TTL choices `5m` and `1h`). GET `/config/list` returns the merged document. POST `/config/update` replaces a namespace. POST `/config/field/update` and POST `/config/field/delete` change one key.

`Overlay(base, db)` copies database keys onto the YAML map. A key present in the database wins. Keys only in YAML stay. `ApplyTyped` then copies `routing_strategy`, `num_retries`, and `timeout` into `RouterSettings` when those keys are present. Absent keys leave the typed values from `config.Load` (default strategy `simple-shuffle`, 2 retries, 60 second timeout).

`ValidateStrategy` still runs on each inference request. Saving an unknown strategy name does not make `Serve` treat it as `simple-shuffle`. The request gets HTTP 400 `unknown routing strategy`.

## What this package does not do

It does not reload `configs/config.yaml` from disk. The file is read once at process start. It does not change a model's price. That is `/price/model` in `gateway/models`.

中文说明见同目录 `readme_cn.md`。
