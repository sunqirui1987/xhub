# gateway/keys

## How to use

Call the HTTP paths and the Go entry points in the sections below. Authenticate with the master key or an admin session unless a route is public.

## Purpose

`keys` is the HTTP API for virtual keys. Operators create a key, list keys, rotate a secret, reset spend, and block a key without sharing the master key. The plaintext secret is returned only by create and regenerate.

## HTTP paths

Send `Authorization: Bearer <master key or admin session>`.

- `POST /key/generate` creates a key. The JSON response includes `key` once.
- `GET /key/list` and `POST /key/list` list keys. Filters include user, team, and alias.
- `GET /key/info` reads one key. Pass the hash or the key alias the handler accepts. The stored hash is not a substitute for the plaintext in later calls.
- `POST /key/update` changes budget, models, and metadata.
- `POST /key/delete` deletes keys.
- `POST /key/regenerate` rotates the secret and returns the new plaintext once.
- `POST /key/block` and `POST /key/unblock` stop or restore use.
- `POST /key/{key}/reset_spend` clears spend for that key.
- `GET /key/aliases` lists aliases for pickers in the dashboard.

## Example

```bash
curl -s http://127.0.0.1:4000/key/generate \
  -H "Authorization: Bearer sk-local-master" \
  -H "Content-Type: application/json" \
  -d '{"key_alias":"ci","models":["gpt-4o-mini"],"max_budget":10}'
```

Use the returned `key` as the bearer token on `/v1/chat/completions`.

## Go callers

The process mounts `keys.Module`. Other packages do not call `Generate` directly unless they implement `keys.Host`, which `*gateway.Server` does.

中文使用说明见同目录的 readme_cn.md。
