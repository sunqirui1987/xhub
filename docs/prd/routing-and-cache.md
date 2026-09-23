# 路由与缓存

## 策略（`router_settings.routing_strategy`）

同一 `model_name` 可对应多个 deployment。每次选一个：

| 策略 | 输入 | 行为 |
|---|---|---|
| `simple-shuffle` | `weight` | 默认；按权重随机 |
| `least-busy` | 进行中请求数 | 最闲 |
| `lowest-cost` | 价格表 input/output | 更便宜 |
| `lowest-latency` / `latency-based-routing` | 滑动延迟 | 更快 |
| `usage-based-routing` / `v2` | TPM/RPM 剩余 | 剩余多者 |
| `tag-based` | 请求 tags | 按 tag 滤池 |
| `auto` / `complexity` / `quality` / `adaptive` | 分类器/质量分 | 自动路由 |

另：budget limiter 排除已超 `max_budget` 的部署。

公共默认：`num_retries=2`、`timeout=60`、`allowed_fails` 触发 **cooldown**（冷却期内不再选该部署）。还有 fallback 模型图、context-window fallback、content-policy fallback、pre-call checks、affinity。

失败：全部冷却或能力不匹配 → **429/400**，message 说明无可用 deployment。未实现供应商 → **`provider_not_implemented`**，禁止当 OpenAI 兼容悄悄转发。

Fallback **每次重新鉴权与限额**（从身份步开始）。首个业务 chunk 已发出则禁止换 Provider。

## DualCache

内存 + Redis（开发可仅内存）。缓存键必须租户隔离（含 hashed key / team）。命中：`cache_hit=true`，头 `x-litellm-cache-key`，仍记 spend（缓存价）。未命中走上游，写回 DualCache。管理面 `POST /flushall` 清空。
