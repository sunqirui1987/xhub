# 系统架构

一个 Go 进程承担治理与供应商转换。

```text
SDK / curl ──▶ 数据面契约 ──▶ Auth → Limits → Guardrail
浏览器    ──▶ 管理契约   ──▶ Router → Adapter → 上游
                              → 计价头 → 异步 Spend / Callbacks
```

| 受众 | 凭证 | 可调用 |
|---|---|---|
| 业务应用 | 虚拟 Key `sk-...` | 数据面 |
| 管理员 / 控制台 | SSO session、master key、`key_type=management` | 管理契约 |

默认 master key 不能调 `/v1/chat/completions`。
