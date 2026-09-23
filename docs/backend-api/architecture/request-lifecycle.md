# 请求生命周期

数据面顺序不可调换有副作用的阶段。

1. 接入，生成 `x-litellm-call-id`
2. 鉴权（见 [auth-decision](auth-decision.md)）
3. 身份：key → user → team → org → project → end_user
4. Pre-call 限额：max_budget、并发、RPM/TPM
5. 入站 Guardrail / Policy；PROXY_HOOKS
6. 路由选 deployment
7. 缓存查找
8. Adapter 转换请求
9. 上游 HTTP/WS；首个业务 chunk 后禁止换供应商重试
10. Adapter 转换响应
11. 出站 Guardrail
12. 计价，写 `x-litellm-response-cost*`
13. 返回调用方
14. 异步 spend flush、callbacks、cooldown

Fallback 每次从第 3 步重新授权。管理 CRUD 不走 Router：鉴权 → 校验 → 写库 → 失效 cache → audit。
