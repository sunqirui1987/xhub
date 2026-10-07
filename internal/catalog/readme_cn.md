# catalog

嵌进二进制的路由和价格说明。JSON 在 `publicdata/`，编译进进程。`init` 调用 `loadPriceDocument` 并记下 `modelCostMapLoadedAt`。解析失败时留下空表，进程仍能启动。

## 路由

`Load` 返回 `routes.json` 的每一行。网关先挂自己的模块（`gateway/routes.go`），再在 `ingress.go` 里遍历这份列表。先登记的方法和路径赢。没有 `"/"` 兜底。模块、这份目录、bypass 端点都没对上的路径，是 Gin `NoRoute` 的 JSON 404。

`IsPublicPath` 是不需要会话就能回答的公开 GET。`PublicBody` 是这个 GET 的固定 JSON。价格行和创建字段来自嵌入文件。其余是空列表。

`IsDataPlanePath` 在混合前缀或 `IsLLMPrefix` 时为真。`IsLLMPrefix` 用 `HasPrefix` 和 `classify.go` 里的前缀比较，包括 `/v1/chat`、`/v1/messages`、`/api/v3/contents`、`/v3/contents`。`HasPrefix` 两边各去掉一个末尾斜杠，所以 `/v1/chat` 不会对上 `/v1/chatcompletions`。`PathMatch` 把 `{name}` 当成一段，把 `:path` 和 `{x:path}` 当成剩余部分。`Split` 按 `/` 拆开，空路径返回 nil，不是含一个 `""` 的切片。

`IsMixedPath` 是旧的共享键值族（`/v1/agents`、`/v1/skills`、`/v1/workflows` 等）。`gateway/family` 的 `ServeMixed` 先要身份，然后拒绝。它们不会从一个全局命名空间里被提供出来。

## 价格

`CostMap` 是模型 id 到字段表。`Cost` 用 `input_cost_per_token` 和 `output_cost_per_token` 乘提示和补全 token。不认识的模型返回 ok 为 false。调用方不能把这次调用存成 0 美元。`KnownProvider` 在前缀是价格表里的供应商 slug 时为真，组织 id（例如 `meta-llama`）不是。通配展开只剥已知的供应商前缀。

`ProviderModels` 返回某个供应商索引到的 id，没有则为 nil。`Count` 是对象行的条数。`LITELLM_LOCAL_MODEL_COST_MAP` 为 `true`（忽略大小写）时 `EnvForced` 为真。

部署上的 `input_cost_per_token` 和 `output_cost_per_token` 在网关记账时盖过这份目录。它们不改写 `pricedata.json`。`price_write.go` 把载入的文档拍成基线。`SetModel` / `RemoveModel` / `SetProvider` / `RemoveProvider` 改内存里的表。`ApplyDocument` 用生成的文档换掉正在用的表，并拒绝空文档，这样一次失败的抓取不会把正在计费的价格抹掉。`adoptBaseline` 接着重新拍照，所以「恢复内置」恢复的是进程正在使用的价格。

`Contribute` 插入一条模型并记住官方 id，之后按这个 id 计费能找到同一费率。id 或供应商为空时什么都不写。

## 这个包不做什么

它不匹配 bypass 请求。那是 Gin 之前的 `provider.Match`。它也不选部署。那是 `router.Order`。

English notes are in `readme.md` in this directory.
