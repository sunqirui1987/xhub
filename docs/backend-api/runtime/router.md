# Router 策略

`router_settings.routing_strategy` 取值与默认：

| 策略 | 输入 | 默认相关 |
|---|---|---|
| `simple-shuffle` | weight | 默认；按 weight 随机 |
| `least-busy` | 进行中请求数 | 选最闲 |
| `lowest-cost` | 价格表 input/output | 选更便宜 |
| `lowest-latency` | 滑动延迟 | 选更快 |
| `usage-based-routing` / `usage-based-routing-v2` | TPM/RPM 剩余 | `lowest_tpm_rpm` |
| `latency-based-routing` | 延迟样本 | 同 lowest-latency |
| `tag-based` | 请求 tags | 按 tag 过滤池 |
| `auto` / `complexity` / `quality` / `adaptive` | 分类器/质量分 | auto_router 等 |
| budget limiter | 主体剩余预算 | 排除超预算部署 |

公共默认：`num_retries=2`、`timeout=60`、`allowed_fails` 触发 cooldown、fallback 模型图、context-window fallback、content-policy fallback、pre-call checks、affinity。每次 fallback 重新授权与预算。
