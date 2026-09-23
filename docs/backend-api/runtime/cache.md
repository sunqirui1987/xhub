# 缓存

DualCache：内存 + Redis。键必须租户隔离。命中标 `cache_hit`，按缓存价记 spend，响应头带 `x-litellm-cache-key`。

后端：in-memory、Redis、disk、S3、GCS、Azure Blob、语义缓存。管理面 `/cache/settings`、`/flushall`、`/ping`。
