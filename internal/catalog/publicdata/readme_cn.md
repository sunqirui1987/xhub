# catalog/publicdata

由 `internal/catalog` 嵌进网关二进制的静态 JSON。进程启动时不从磁盘读这些文件。写入方是测试和 `./cmd/pricedata`。

| 文件 | 是什么 |
| --- | --- |
| `pricedata.json` | 从 Modelink 市场接口 `https://api.modelink.ai/v1/market/models` 生成的模型价格行。每个键都是真实模型 id。没有 LiteLLM 的样例行，也没有 `sample_spec`。 |
| `routes.json` | 网关在自己的模块之后挂上的 HTTP 目录。`catalog.Load` 返回这些行。不在这个文件里、也不在模块上的路径是 JSON 404。 |
| 旁边的供应商和字段 JSON | 添加模型时的公开载荷：端点类型在 Go 里登记（`internal/provider`），这个目录放生成好的价格和路由文档。 |

`catalog.loadPriceDocument` 把 `pricedata.json` 解析一次。解析失败时留下空表，进程仍能启动；此时 `Cost` 返回 ok 为 false，调用方不能把这次调用记成免费。`LITELLM_LOCAL_MODEL_COST_MAP=true` 强制只用内置表，不再尝试远程价格源。

手改的价格不会重写这个文件。`catalog/price_write.go` 留一份基线快照，再把数据库里的覆盖盖在上面。清掉覆盖就恢复基线那一行。

English notes are in `readme.md` in this directory.
