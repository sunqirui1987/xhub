# config

## Purpose

`config` loads the YAML file the gateway process starts with. It is the only supported way to turn `configs/config.yaml` into a `Config` value. SQLite and an empty database URL are rejected so the process cannot silently fall back to a local file database.

## Features

- `Load` reads YAML and resolves `os.environ/NAME` placeholders in the master key, database URL, Redis URL, and the configured administrator credentials.
- Typed `RouterSettings` and `GeneralSettings` cover the fields the process reads directly.
- `RouterRaw` and `GeneralRaw` keep every other YAML key so a database overlay can replace one key without dropping the rest.
- `ModelEntry.ParamString` reads one upstream parameter with a fallback.
- `SplitProviderModel` splits a `provider/model` string.

## The configured administrator

`general_settings.admin_email` and `admin_password` name the first platform administrator. The gateway creates that account at startup when no account with the address exists yet, so a fresh deployment does not have to call `POST /bootstrap` by hand.

The password is an **initial** password, not a managed one:

- the account is created only when the address is unknown
- a later change to the value never rewrites a stored password
- changing a live password goes through the account routes, not through a config file

Both values accept `os.environ/NAME`, which is how a deployment keeps the password out of a file. An unset variable resolves to the empty string, and an empty `admin_email` or `admin_password` means seeding does not run at all. `disable_env_credential_login: true` refuses the config-supplied account entirely.

```yaml
general_settings:
  master_key: sk-local-master
  database_url: postgres://xhub:xhub@127.0.0.1:5433/xhub?sslmode=disable
  admin_email: admin@example.com
  admin_password: os.environ/XHUB_ADMIN_PASSWORD
  admin_name: Platform Admin
  disable_env_credential_login: false
```

## How another package uses it

Import `github.com/sunqirui1987/xhub/internal/config`.

```go
cfg, err := config.Load("configs/config.yaml")
if err != nil {
    log.Fatal(err)
}
iamDB, err := iam.Open(ctx, cfg.GeneralSettings.DatabaseURL)
srv := gateway.New(cfg, st, iamDB)
```

A model entry looks like this in YAML:

```yaml
model_list:
  - model_name: gpt-4o-mini
    litellm_params:
      model: openai/gpt-4o-mini
      api_key: os.environ/OPENAI_API_KEY
      api_base: https://api.openai.com/v1
router_settings:
  routing_strategy: simple-shuffle
```

`Load` returns an error when `database_url` is missing or starts with `sqlite:` or `file:`.

## What this package does not do

It does not merge database overrides. That is `settings.Overlay`. It does not open PostgreSQL.

中文使用说明见同目录的 readme_cn.md。
