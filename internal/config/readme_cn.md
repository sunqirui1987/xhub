# config

## 这个模块做什么

`config` 读取网关进程启动时的 YAML。把 `configs/config.yaml` 变成 `Config` 只走这里。空的数据库地址和 SQLite 会被拒绝，进程不会悄悄退回本地文件库。

## 功能

- `Load` 读 YAML，并把主密钥、数据库地址、Redis 地址、以及配置里的管理员凭据中的 `os.environ/NAME` 换成环境变量。
- `RouterSettings` 和 `GeneralSettings` 覆盖进程直接读取的字段。
- `RouterRaw` 和 `GeneralRaw` 保留其余 YAML 键，数据库覆盖某一键时不会丢掉其它键。
- `ModelEntry.ParamString` 读取一个上游参数，没有时用你给的默认值。
- `SplitProviderModel` 拆开 `provider/model`。

## 配置里的默认管理员

`general_settings.admin_email` 和 `admin_password` 指定首位平台管理员。启动时如果该邮箱还没有账号，网关就创建它，因此新部署不必手动调 `POST /bootstrap`。

这个密码是**初始密码**，不是被托管的密码：

- 只在邮箱未知时创建账号
- 之后改这个值**不会**重写已存的密码
- 改线上密码走账号接口，不走配置文件

两个值都支持 `os.environ/NAME`，部署时用它把密码留在文件之外。变量没设时解析成空串，而 `admin_email` 或 `admin_password` 为空就完全不执行创建。`disable_env_credential_login: true` 则彻底拒绝配置提供的账号。

```yaml
general_settings:
  master_key: sk-local-master
  database_url: postgres://xhub:xhub@127.0.0.1:5433/xhub?sslmode=disable
  admin_email: admin@example.com
  admin_password: os.environ/XHUB_ADMIN_PASSWORD
  admin_name: Platform Admin
  disable_env_credential_login: false
```

## 其它包怎么用

导入 `github.com/sunqirui1987/xhub/internal/config`。

```go
cfg, err := config.Load("configs/config.yaml")
if err != nil {
    log.Fatal(err)
}
iamDB, err := iam.Open(ctx, cfg.GeneralSettings.DatabaseURL)
srv := gateway.New(cfg, st, iamDB)
```

YAML 里一个模型是这样写的：

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

`database_url` 为空，或者以 `sqlite:`、`file:` 开头时，`Load` 返回错误。

## Load 实际改了什么

YAML 解码之后，`Load` 遍历每条部署 `litellm_params` 里的字符串。以 `os.environ/` 开头的值换成 `os.Getenv`。变量不存在时变成空串，于是这项配置保持关闭，而不是把占位符发给上游。这里不把 `custom_llm_provider` 转成小写；那一步在 `internal/llm` 灌凭据时做。

空的 `routing_strategy` 变成 `simple-shuffle`。`num_retries` 为 0 时变成 2。`timeout` 为 0 时变成 60 秒。这些默认值在 `Load` 里写上，不是路由器写的。

`ModelEntry.ParamString(key, fallback)` 从 `litellm_params` 读一个字符串。缺键或不是字符串时返回 `fallback`。`router.DeploymentID` 用它读 `api_base` 和 `model`，再用 `|` 拼成部署 id。

## 这个包不做什么

它不合并数据库里的覆盖值。那是 `internal/gateway/prefs` 的覆盖。它不打开 PostgreSQL。它也不监听 `:4000`。网关进程在启动时按命令行给出的路径调用一次 `Load`。
