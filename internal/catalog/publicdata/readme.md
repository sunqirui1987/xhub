# catalog/publicdata

## Purpose

This directory holds the JSON documents that `catalog` embeds into the gateway binary. They are data, not Go code. Changing a price or a dashboard form field means editing the JSON here and rebuilding. The running process does not watch these files on disk.

## Files

- `model_cost_map.json` is the built-in price map. `catalog.CostMap` reads it. A model missing from this file must not be priced as zero.
- `autorouter_presets.json` is the preset list the dashboard shows for auto routers.
- `provider_create_fields.json` describes the fields the dashboard renders when an operator adds a provider deployment.
- `agent_create_fields.json` describes the fields for the agent create form. The agent product surface is not mounted, but the document remains part of the embedded catalog so the field definitions stay with the other public data.

## How to change the data

Edit the JSON, keep it valid, and rebuild `./cmd/gateway`. Callers do not open these files with `os.ReadFile`. They call `catalog.CostMap`, `catalog.PublicBody`, or the reload helpers in `gateway/models`.

Do not add comments inside the JSON. Keep identifiers stable: the dashboard and the price estimator look up model names and field names exactly.

中文使用说明见同目录的 readme_cn.md。
