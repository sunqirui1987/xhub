# gateway/usage

## How to use

Call the HTTP paths and the Go entry points in the sections below. Authenticate with the master key or an admin session unless a route is public.

## Purpose

`usage` serves the spend and activity screens. Numbers come from spend rows already stored in PostgreSQL. Opening the page does not recompute prices. When no auto-router benchmark sample exists, the benchmark route returns an empty list rather than an invented score.

## HTTP paths

- `GET /spend/logs/v2` lists request logs. `cache_hit` is a boolean. One log is `GET /spend/logs/ui/{request_id}`.
- `GET /global/spend/keys`, `/global/spend/models`, `/global/spend/provider`, `/global/spend/teams`, and `/global/spend/tags` group spend for the charts.
- `GET /global/activity` and `/global/activity/cache_hits` feed the activity panels.
- `POST /spend/calculate` estimates a cost from the token counts you send. It does not write a log row.
- `GET /auto_router/benchmarks` returns stored benchmark rows, or an empty list.

## Example

```bash
curl -s "http://127.0.0.1:4000/spend/logs/v2?start_date=2026-09-01&end_date=2026-09-30" \
  -H "Authorization: Bearer sk-local-master"
```

An admin sees every user's spend. A non-admin is limited to their own user id.

## What this package does not do

It does not price an in-flight chat call. That happens in the data plane when the response is recorded.

中文使用说明见同目录的 readme_cn.md。
