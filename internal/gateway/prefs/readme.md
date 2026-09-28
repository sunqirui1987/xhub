# gateway/prefs

## How to use

Call the HTTP paths and the Go entry points in the sections below. Authenticate with the master key or an admin session unless a route is public.

## Purpose

`prefs` serves the router-settings and general-settings screens. The value you read is the YAML baseline overlaid with the database row. A key present in the database wins, even when the value is a list or null. A key absent from the database stays at the YAML value.

## HTTP paths

- `GET /router/settings` returns the merged router document the dashboard renders. `GET /router/fields` returns the same document for the field editor.
- `POST /config/update` writes one field into the database overlay. Lists and scalars replace. Nested objects merge.
- `GET /config/list` returns the general-settings view. `master_key`, `database_url`, and `redis_url` are not copied into that view.
- Callback listing for the logging screen is mounted next to these routes.

## Example

```bash
curl -s http://127.0.0.1:4000/config/update \
  -H "Authorization: Bearer sk-local-master" \
  -H "Content-Type: application/json" \
  -d '{"router_settings":{"routing_strategy":"least-busy"}}'
```

Then `GET` the router settings route again. `routing_strategy` is `least-busy`, and fields you did not send are still present.

## Go callers

`prefs.MergedRouter` and `prefs.MergedGeneral` return the same documents the HTTP handlers write. `Overlay` puts database keys on top of YAML. `MergePatch` folds a partial update into the current document. Pass a host that can read config and the store. The process implements that host.

中文使用说明见同目录的 readme_cn.md。
