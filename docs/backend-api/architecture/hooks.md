# PROXY_HOOKS

网关预调用钩子注册表名为 **PROXY_HOOKS**。必须实现下列 key（以及 `hooks/` 目录中未进表的模块）：

| PROXY_HOOKS key | 作用 |
|---|---|
| `max_budget_limiter` | max_budget / soft_budget |
| `parallel_request_limiter` | 并发；`LEGACY_MULTI_INSTANCE_RATE_LIMITING` 时用 v1 |
| `cache_control_check` | 允许的 cache 指令 |
| `responses_id_security` | Responses id 所有权 |
| `litellm_skills` | Skills 注入 |
| `max_iterations_limiter` | agent 迭代上限 |
| `max_budget_per_session_limiter` | 会话预算 |
| `sensitive_data_routing` | 敏感数据路由 |
| `prompt_cache_prediction` | prompt cache 预测 |

目录内其它模块同样要有等价行为：`dynamic_rate_limiter_v3`、`batch_rate_limiter`、`prompt_injection_detection`、`azure_content_safety`、`mcp_semantic_filter`、`model_max_budget_limiter`、`key_management_event_hooks`、`user_management_event_hooks`。
