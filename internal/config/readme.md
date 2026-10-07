# config

## Purpose

`config` loads the YAML file the gateway process starts with. It is the only supported way to turn `configs/config.yaml` into a `Config` value. SQLite and an empty database URL are rejected so the process cannot silently fall back to a local file database.

## Features

- `Load` reads YAML and resolves `os.environ/NAME` placeholders, after trimming space, in `master_key`, `database_url`, `redis_url`, `admin_email`, `admin_password`, and `admin_name`.
- Typed `RouterSettings` and `GeneralSettings` cover the fields the process reads directly.
- `RouterRaw` and `GeneralRaw` keep every other YAML key so a database overlay can replace one key without dropping the rest.
- `ModelEntry.ParamString` reads one upstream parameter with a fallback.
- `SplitProviderModel` splits on the first `/`. The left side is the provider and the right side is the model. A string with no slash returns provider `openai` and the whole string as the model. It does not return an empty provider.

## The configured administrator

`general_settings.admin_email` and `admin_password` name the first platform administrator. The gateway creates that account at startup when no account with the address exists yet, so a fresh deployment does not have to call `POST /bootstrap` by hand.

The password is an **initial** password, not a managed one:

- the account is created only when the address is unknown
- a later change to the value never rewrites a stored password
- changing a live password goes through the account routes, not through a config file

Both values accept `os.environ/NAME`, which is how a deployment keeps the password out of a file. An unset variable resolves to the empty string, and an empty `admin_email` or `admin_password` means seeding does not run at all. `disable_env_credential_login: true` refuses the config-supplied account entirely.

The three files in the tree are not the same URL:

| File | Who reads it | `database_url` |
| --- | --- | --- |
| `configs/config.yaml` | `make run` | `postgres://xhub:xhub_dev_password@127.0.0.1:5433/xhub?sslmode=disable` |
| `configs/config.example.yaml` | nobody; it is a template | `postgres://xhub:xhub@127.0.0.1:5432/xhub?sslmode=disable` |
| `configs/config.docker.yaml` | the gateway container | `postgres://xhub:xhub_dev_password@postgres:5432/xhub?sslmode=disable` |

Compose publishes container port `5432` as host port `5433`. The example file's port `5432` and password `xhub` do not open that database.

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

## What Load actually rewrites

After YAML decode, `Load` walks every string in each deployment's `litellm_params`. A value that starts with `os.environ/` is replaced with `os.Getenv`. A missing variable becomes the empty string, so the feature that key configures stays off instead of sending the placeholder upstream. `custom_llm_provider` is not lowercased here; credential hydration in `internal/llm` does that later.

An empty `routing_strategy` becomes `simple-shuffle`. `num_retries` of 0 becomes 2. `timeout` of 0 becomes 60 seconds. Those defaults are applied in `Load`, not by the router.

`ModelEntry.ParamString(key, fallback)` reads one string from `litellm_params`. A missing key or a non-string returns `fallback`. `router.DeploymentID` uses it for `api_base` and `model`, and joins them with `|`.

## What this package does not do

It does not merge database overrides. That is the prefs overlay in `internal/gateway/prefs`. It does not open PostgreSQL. It does not listen on `:4000`. The gateway process calls `Load` once at startup from the path in the process flags.

中文使用说明见同目录的 readme_cn.md。
