# config

## Purpose

`config` loads the YAML file the gateway process starts with. It is the only supported way to turn `configs/config.yaml` into a `Config` value. SQLite and an empty database URL are rejected so the process cannot silently fall back to a local file database.

## Features

- `Load` reads YAML and resolves `os.environ/NAME` placeholders in the master key, database URL, and Redis URL.
- Typed `RouterSettings` and `GeneralSettings` cover the fields the process reads directly.
- `RouterRaw` and `GeneralRaw` keep every other YAML key so a database overlay can replace one key without dropping the rest.
- `ModelEntry.ParamString` reads one upstream parameter with a fallback.
- `SplitProviderModel` splits a `provider/model` string.

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/config`.

```go
cfg, err := config.Load("configs/config.yaml")
if err != nil {
    log.Fatal(err)
}
st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
srv := gateway.New(cfg, st)
```

A model entry looks like this in YAML:

```yaml
model_list:
  - model_name: gpt-4o-mini
    litellm_params:
      model: openai/gpt-4o-mini
      api_key: os.environ/OPENAI_API_KEY
      api_base: https://api.openai.com/v1
general_settings:
  master_key: sk-local-master
  database_url: postgres://xhub:xhub@127.0.0.1:5433/xhub?sslmode=disable
router_settings:
  routing_strategy: simple-shuffle
```

`Load` returns an error when `database_url` is missing or starts with `sqlite:` or `file:`.

## What this package does not do

It does not merge database overrides. That is `settings.Overlay`. It does not open PostgreSQL.

中文使用说明见同目录的 readme_cn.md。
