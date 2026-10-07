# gateway/prefs

存在 PostgreSQL 里、盖在 YAML `Config` 上的路由和通用设置。以 `prefs.Module` 挂上。

GET `/router/settings` 和 GET `/router/fields` 给出设置页文档：策略、重试、超时，以及字段目录（`generalFieldCatalog` 里提示缓存 TTL 的选项是 `5m` 和 `1h`）。GET `/config/list` 返回合并后的文档。POST `/config/update` 替换一个命名空间。POST `/config/field/update` 和 POST `/config/field/delete` 改一个键。

`Overlay(base, db)` 把数据库里的键抄到 YAML 表上。数据库里有的键赢。只在 YAML 里的键保留。`ApplyTyped` 再在这些键存在时把 `routing_strategy`、`num_retries`、`timeout` 抄进 `RouterSettings`。缺的键留着 `config.Load` 写上的类型化值（默认策略 `simple-shuffle`、2 次重试、60 秒超时）。

每次推理请求仍会跑 `ValidateStrategy`。存了一个不认识的策略名，不会让 `Serve` 把它当成 `simple-shuffle`。请求得到 HTTP 400 `unknown routing strategy`。

## 这个包不做什么

它不从磁盘重读 `configs/config.yaml`。这个文件在进程启动时读一次。它也不改模型价格。那是 `gateway/models` 的 `/price/model`。

English notes are in `readme.md` in this directory.
