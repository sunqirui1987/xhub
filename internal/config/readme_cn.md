# config

## 这个模块做什么

`config` 读取网关进程启动时的 YAML。把 `configs/config.yaml` 变成 `Config` 只走这里。空的数据库地址和 SQLite 会被拒绝，进程不会悄悄退回本地文件库。

## 功能

- `Load` 读 YAML，并把主密钥、数据库地址、Redis 地址里的 `os.environ/NAME` 换成环境变量。
- `RouterSettings` 和 `GeneralSettings` 覆盖进程直接读取的字段。
- `RouterRaw` 和 `GeneralRaw` 保留其余 YAML 键，数据库覆盖某一键时不会丢掉其它键。
- `ModelEntry.ParamString` 读取一个上游参数，没有时用你给的默认值。
- `SplitProviderModel` 拆开 `provider/model`。

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/config`。

```go
cfg, err := config.Load("configs/config.yaml")
if err != nil {
    log.Fatal(err)
}
st, err := store.Open(cfg.GeneralSettings.DatabaseURL)
srv := gateway.New(cfg, st)
```

YAML 里一个模型是这样写的：

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

`database_url` 为空，或者以 `sqlite:`、`file:` 开头时，`Load` 返回错误。

## 这个包不做什么

它不合并数据库里的覆盖值。那是 `settings.Overlay`。它也不打开 PostgreSQL。
