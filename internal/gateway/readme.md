# gateway

## Purpose

`gateway` is the process. It owns configuration, PostgreSQL, Redis, the Gin engine, sessions, and the module list. External programs start it. Internal packages do not import it; the process implements their Host and Gate interfaces in `wire.go`.

## Features

- `New` builds a `Server` from a `Config` and a `Store`. `Run` listens and starts the spend flush when Redis is set.
- `Handler` is the `http.Handler` to mount in tests or behind another server. `GinRoutes` lists what was registered. There is no `/` catch-all.
- `Use` mounts an `httpx.Module` after startup. A duplicate module name fails. The first method and path wins.
- `RequireManage` and `RequireLLMPrincipal` are the auth doors for admin routes and inference routes.
- `IsRemovedColumn` reports paths that stay 404, including the removed agent, MCP, and skill surfaces.
- `SetUIProxy` installs the dashboard reverse proxy. Nil means the console is not running.
- Health routes: `GET /health/liveliness` returns `{"status":"ok"}` without a database ping. Readiness pings PostgreSQL.

## How you run it

From the repository root:

```bash
go build -o xhub ./cmd/gateway
./xhub -config configs/config.yaml -addr 127.0.0.1:4000
curl -s http://127.0.0.1:4000/health/liveliness
```

From Go:

```go
cfg, err := config.Load("configs/config.yaml")
st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
srv := gateway.New(cfg, st)
if err := srv.Use(someModule); err != nil {
    log.Fatal(err)
}
log.Fatal(srv.Run("127.0.0.1:4000"))
```

Call the inference API with the master key or a virtual key:

```bash
curl -s http://127.0.0.1:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-local-master" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
```

Dashboard login is `POST /login` or `POST /v2/login` with the UI username and password. The default username is `admin`. The default password is the master key unless `UI_USERNAME` and `UI_PASSWORD` are set.

## What lives in the child packages

Keys, models, guardrails, identity, usage, settings, and the catalog family are mounted as modules. Their READMEs describe the HTTP paths. This directory is the process that calls them.

中文使用说明见同目录的 readme_cn.md。
