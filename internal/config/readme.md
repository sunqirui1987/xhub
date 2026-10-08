# Startup configuration and model definitions

[简体中文](readme_cn.md) · [Feature implementation reference](../../docs/development/implementation.md)

## Responsibilities and behavior

config.go loads YAML into Config and ModelEntry, resolving os.environ/NAME references. Unknown router and general settings survive round trips without implying runtime support. Database configuration accepts PostgreSQL URLs only.
Load seeds simple-shuffle, two attempts, and a 60-second timeout. These startup defaults differ from typed runtime readers for missing template fields. ModelEntry.Disabled checks the boolean model_info.disabled value; discovery and routing exclude disabled deployments.
An initial administrator password seeds an absent account once, rather than overwriting a persisted password at each startup. The optional master credential is not a login credential. Keep resolved provider secrets out of documentation, responses, and logs.

## Source responsibilities and entry points

### config.go

Exported types: `Config`, `ModelEntry`, `RouterSettings`, `GeneralSettings`.

- [`func (m ModelEntry) Disabled() bool`](config.go) — Disabled reports whether this deployment is excluded from model discovery and runtime routing.
- [`func Load(path string) (*Config, error)`](config.go) — Load reads YAML and requires a postgres:// or postgresql:// database URL.
- [`func (e ModelEntry) ParamString(key, fallback string) string`](config.go) — ParamString 从一条部署的 litellm_params 里按名字读字符串。缺键或类型不对时返回 fallback。
- [`func SplitProviderModel(raw string) (provider, model string)`](config.go) — SplitProviderModel splits provider/model. With no slash the provider is empty and the model name is the whole string.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../logx/readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [config_test.go](config_test.go) | `TestLoadReadsTheConfiguredAdministrator`, `TestLoadResolvesTheAdministratorPasswordFromTheEnvironment`, `TestLoadDoesNotRequireAMasterKey`, `TestLoadWithoutAnAdministratorStillSucceeds`, `TestLoadReadsDisableEnvCredentialLogin` |
| [url_test.go](url_test.go) | `TestDatabaseURLRequiresPostgresScheme` |

```bash
go test ./internal/config -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
