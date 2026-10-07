# catalog/publicdata

由 `internal/catalog` 嵌进网关二进制的静态 JSON。进程启动时不从磁盘读这些文件。写入方是测试和 `./cmd/pricedata`。

这个目录只有两个 JSON 文件：`pricedata.json` 和 `autorouter_presets.json`。这里没有 `routes.json`，也没有供应商或字段 JSON。

| 文件 | 是什么 |
| --- | --- |
| `pricedata.json` | 从 Modelink 市场接口 `https://api.modelink.ai/v1/market/models` 生成的模型价格行。每个键都是真实模型 id。没有 LiteLLM 的样例行，也没有 `sample_spec`。`catalog/embed.go` 用 `//go:embed publicdata/pricedata.json` 嵌进二进制。`loadPriceDocument` 在启动时把这些字节解析一次。 |
| `autorouter_presets.json` | 路径含 `/public/autorouter_presets` 的公开 GET 用的预设文档。同一份 `embed.go` 用 `//go:embed publicdata/autorouter_presets.json` 嵌入。`PublicBody` 解析它。解析失败时返回 `{"presets": []}`。 |

`routes.json` 是 `internal/catalog/routes.json`，由 `catalog/embed.go` 的 `//go:embed routes.json` 嵌入。`catalog.Load` 返回这些行。模块、这份目录、bypass 端点都没对上的路径，是 Gin `NoRoute` 的 JSON 404。

`catalog.loadPriceDocument` 把 `pricedata.json` 解析一次。解析失败时留下空表，进程仍能启动；此时 `Cost` 返回 ok 为 false，调用方不能把这次调用记成免费。

`LITELLM_LOCAL_MODEL_COST_MAP=true`（忽略大小写，并去掉两端空白）时 `catalog.EnvForced` 返回 true。`gateway/models.CostMapSource` 把这个布尔值抄进 GET `/model/cost_map/source` 的 `is_env_forced`。`gateway/catalog.go` 还把 `EnvForced` 包成 `localCostMapForced`，没有任何调用方走到这个包装。这个变量不会把正在用的价格表换回嵌入文件，也不会跳过远程抓取。`models.ReloadCostMap` 和 `models.scheduledReloadLoop` 不读它。两者都调用 `reloadNow`，而 `reloadNow` 总会调用 `catalog.ReloadFromMarket`。

手改的价格不会重写这个文件。`catalog/price_write.go` 留一份基线快照，再把数据库里的覆盖盖在上面。清掉覆盖就恢复基线那一行。

English notes are in `readme.md` in this directory.
