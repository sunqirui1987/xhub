# gateway/models

## How to use

Call the HTTP paths and the Go entry points in the sections below. Authenticate with the master key or an admin session unless a route is public.

## Purpose

`models` lists the models a caller may use and stores dashboard edits. A model saved in PostgreSQL overrides a YAML entry with the same public name. Wildcards such as `openai/*` are expanded in the list the caller actually sees.

## HTTP paths

- `GET /v1/models` and `GET /models` list models visible to the bearer token.
- `POST /model/new` adds a deployment. `POST /model/update` edits it. `POST /model/delete` removes the database row.
- `POST /model/block` and `POST /model/unblock` hide or restore a model.
- `GET /model/info` returns the public deployment record.
- `POST /reload/model_cost_map` reloads prices immediately. The dashboard reads `status`, `models_count`, and `scheduled`.
- `POST /schedule/model_cost_map_reload` and `DELETE /schedule/model_cost_map_reload` arm or cancel a timed reload. `GET /schedule/model_cost_map_reload/status` reports the timer.

## Example

```bash
curl -s http://127.0.0.1:4000/v1/models \
  -H "Authorization: Bearer sk-local-master"
```

A virtual key with `models: ["gpt-4o-mini"]` sees only that name, not the full catalog.

## Go callers

`models.Module` is mounted by the process. `models.List` is the handler behind the public model list. Do not query `catalog.CostMap` to decide what a key may call. The allow-list on the key, team, and user is applied in this package.

中文使用说明见同目录的 readme_cn.md。
