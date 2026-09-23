# 术语

| 术语 | 含义 |
|---|---|
| VerificationToken | Key 的库实体；`token` 列为哈希 |
| `key_type` | `llm_api` / `management` / `read_only` / `default` |
| 虚拟 Key | 发给调用方的 `sk-…`，库中哈希，明文只展示一次 |
| Deployment | `model_list` 一条上游；`model_name` 是客户端别名 |
| Router | 多 deployment 间选择、重试、fallback、冷却 |
| Spend | 按 token/请求计量的 USD 费用 |
| Guardrail | 请求前/后内容策略 |
| Passthrough | 按供应商原生路径转发 |
| MCP | Model Context Protocol |
| A2A | Agent-to-Agent |
| Access Group | 模型访问组 |
| Playground | 管理员调试台 |
| Chat 壳 | 终端用户对话 UI |
| PROXY_HOOKS | 网关预调用钩子注册表（含 `max_budget_limiter`） |
| DualCache | 内存+Redis；命中 `cache_hit` |
| `provider_not_implemented` | 未实现供应商的强制错误码 |
