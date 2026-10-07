# catalog

嵌进二进制的路由和价格说明。`routes.json` 是 `internal/catalog/routes.json`，由 `embed.go` 的 `//go:embed routes.json` 编进进程。`publicdata/` 只有 `pricedata.json` 和 `autorouter_presets.json`。`init` 调用 `loadPriceDocument` 并记下 `modelCostMapLoadedAt`。解析失败时留下空表，进程仍能启动。

## 路由

`Load` 返回 `routes.json` 的每一行。网关先挂自己的模块（`gateway/routes.go`），再在 `ingress.go` 里遍历这份列表。先登记的方法和路径赢。没有 `"/"` 兜底。模块、这份目录、bypass 端点都没对上的路径，是 Gin `NoRoute` 的 JSON 404。

`IsPublicPath` 是不需要会话就能回答的公开 GET。`PublicBody` 按小写路径分支，顺序如下：

- 路径含 `/public/agents/fields` 时返回写死的空列表。代理产品面没有挂上；这条路径留着，调用方得到 `[]`，而不是 404。
- 路径含 `/public/providers/fields` 时返回 `mergeProviders(Providers())`。`Providers` 是从价格文档载入的供应商列表（`baseProviders`），不是 `publicdata/` 里的 JSON 文件。
- 路径含 `/public/autorouter_presets` 时解析嵌入的 `publicdata/autorouter_presets.json`。解析失败返回 `{"presets": []}`。
- 路径含 `blog_posts` 时返回 `{"posts": []}`。
- 路径含 `scorer_defaults` 时返回一个表，其中 `tier_boundaries`、`token_thresholds`、`dimension_weights` 都是空对象。
- 路径含 `model_cost_map` 时返回正在用的 `modelCostMapValue`；该值为 nil 时返回 `{}`。这张表启动时来自嵌入的 `pricedata.json`，之后 `ReloadFromMarket` 可以换掉它。
- 路径含 `skill_hub` 时返回 `{"plugins": [], "count": 0}`。
- 路径含 `model_hub/info` 时返回 `{"data": []}`。这一支排在 hub 分支前面，所以这条 info 路径不是空列表。
- 路径含 `model_hub`、`agent_hub` 或 `mcp_hub` 时返回 `[]`。
- 路径以 `/fields` 或 `/providers` 结尾时返回 `[]`。
- 其余返回 `{"object": "list", "data": []}`。

创建字段不是每一个公开 GET 都去读同一份嵌入文件。默认值是一张表，不是空列表。

`IsDataPlanePath` 在混合前缀或 `IsLLMPrefix` 时为真。`IsLLMPrefix` 用 `HasPrefix` 和 `classify.go` 里的前缀比较，包括 `/v1/chat`、`/v1/messages`、`/api/v3/contents`、`/v3/contents`。`HasPrefix` 两边各去掉一个末尾斜杠，所以 `/v1/chat` 不会对上 `/v1/chatcompletions`。`PathMatch` 把 `{name}` 当成一段，把 `:path` 和 `{x:path}` 当成剩余部分。`Split` 按 `/` 拆开，空路径返回 nil，不是含一个 `""` 的切片。

`IsMixedPath` 是旧的共享键值族（`/v1/agents`、`/v1/skills`、`/v1/workflows` 等）。`gateway/family` 的 `ServeMixed` 先要身份，然后拒绝。它们不会从一个全局命名空间里被提供出来。

## 价格

`CostMap` 是模型 id 到字段表。`Cost` 用 `input_cost_per_token` 和 `output_cost_per_token` 乘提示和补全 token。不认识的模型返回 ok 为 false。调用方不能把这次调用存成 0 美元。`KnownProvider` 在前缀是价格表里的供应商 slug 时为真，组织 id（例如 `meta-llama`）不是。通配展开只剥已知的供应商前缀。

`ProviderModels` 返回某个供应商索引到的 id，没有则为 nil。`Count` 是对象行的条数。`LITELLM_LOCAL_MODEL_COST_MAP` 为 `true`（忽略大小写，并去掉两端空白）时 `EnvForced` 为真。`models.CostMapSource` 把这个布尔值抄到 GET `/model/cost_map/source` 的 `is_env_forced`。`gateway/catalog.go` 把它包成 `localCostMapForced`，没有任何调用方走到这个包装。这个变量不会拦住 `ReloadCostMap` 或 `scheduledReloadLoop`：两者都不读它，并且都调用 `reloadNow`，而 `reloadNow` 总会调用 `ReloadFromMarket`。

部署上的 `input_cost_per_token` 和 `output_cost_per_token` 在网关记账时盖过这份目录。它们不改写 `pricedata.json`。`price_write.go` 把载入的文档拍成基线。`SetModel` / `RemoveModel` / `SetProvider` / `RemoveProvider` 改内存里的表。`ApplyDocument` 用生成的文档换掉正在用的表，并拒绝空文档，这样一次失败的抓取不会把正在计费的价格抹掉。`adoptBaseline` 接着重新拍照，所以「恢复内置」恢复的是进程正在使用的价格。

`Contribute` 插入一条模型并记住官方 id，之后按这个 id 计费能找到同一费率。id 或供应商为空时什么都不写。

## 这个包不做什么

它不匹配 bypass 请求。那是 Gin 之前的 `provider.Match`。它也不选部署。那是 `router.Order`。

English notes are in `readme.md` in this directory.
