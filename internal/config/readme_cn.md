# 启动配置与模型定义

[English](readme.md) · [全功能实现说明](../../docs/development/implementation.md)

## 职责与实现契约

config.go 定义 Config、ModelEntry，加载 YAML 并解析 os.environ/NAME 引用。未知 router_settings 和 general_settings 字段保留，便于配置往返，但保存不意味着数据面一定执行。数据库地址只支持 PostgreSQL URL。
Load 的平台默认策略是 simple-shuffle，重试预填 2，超时 60 秒；这些启动默认与 RouteSettings 对缺失字段的运行时读取规则要分开说明。ModelEntry.Disabled 从 model_info.disabled 的布尔值判断暂停，模型发现和运行时选路均排除暂停部署。
管理员初始密码只用于创建尚不存在的账号；后续修改配置不覆盖已持久化密码。master 可选且不是登录凭据。命名凭据和内联历史凭据需按协议解析；环境变量不应写入响应、README 或日志。

## 源码职责与入口

### config.go

公开类型：`Config`, `ModelEntry`, `RouterSettings`, `GeneralSettings`.

- [`func (m ModelEntry) Disabled() bool`](config.go) — Disabled reports whether this deployment is excluded from model discovery and runtime routing.
- [`func Load(path string) (*Config, error)`](config.go) — Load reads YAML and requires a postgres:// or postgresql:// database URL.
- [`func (e ModelEntry) ParamString(key, fallback string) string`](config.go) — ParamString 从一条部署的 litellm_params 里按名字读字符串。缺键或类型不对时返回 fallback。
- [`func SplitProviderModel(raw string) (provider, model string)`](config.go) — SplitProviderModel splits provider/model. With no slash the provider is empty and the model name is the whole string.

## 对外 HTTP 边界

无本目录直接登记的 HTTP 路由。导出的 Go API 由上层调用；运行时目录调度或调用宿主的入口应沿依赖链追踪。

## 依赖关系

[internal/logx](../logx/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [config_test.go](config_test.go) | `TestLoadReadsTheConfiguredAdministrator`, `TestLoadResolvesTheAdministratorPasswordFromTheEnvironment`, `TestLoadDoesNotRequireAMasterKey`, `TestLoadWithoutAnAdministratorStillSucceeds`, `TestLoadReadsDisableEnvCredentialLogin` |
| [url_test.go](url_test.go) | `TestDatabaseURLRequiresPostgresScheme` |

```bash
go test ./internal/config -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
