# 怎么运转：两平面与一次请求

XHub 一个进程对外两套 HTTP。调用方用错平面或错凭证，按下面失败，不要混成一个「内部 API」。

## 数据面 vs 管理面

| | 数据面 | 管理面 |
|---|---|---|
| 谁 | SDK / curl（API 调用方） | 控制台、管理员脚本 |
| 凭证 | 虚拟 Key `sk-…`（`key_type` 为 `llm_api` 或 `default`） | master key、SSO cookie、或 `key_type=management` |
| 例子 | `POST /v1/chat/completions` | `POST /key/generate`、`GET /team/list` |
| 失败 | 无/错 Key → **401** OpenAI 信封；超 `max_budget` / RPM / TPM → **429** + `Retry-After` | 无管理权限 → **401/403** |

Master key **默认不能**调 `/v1/chat/completions`（以及其它数据面）。若用 master key 打 Chat：401，`error.type` 按鉴权失败，而不是当成无限额虚拟 Key。除非 `general_settings` 显式打开 master-as-llm。

SDK 只改 `base_url` + `api_key`。禁止要求自有必填头才能 Chat。

## 数据面阶段顺序（不可调换有副作用的步）

一次 `POST /v1/chat/completions`（及其它数据面）必须按此顺序。缺步或换序会导致超卖预算或错误重试。

1. **接入**  
   生成 `x-litellm-call-id`（响应头原样带回）。读 JSON / multipart / 升级 WebSocket。
2. **鉴权**  
   见 [auth-and-keys.md](auth-and-keys.md)。失败：**401**，信封 `{"error":{"message","type","code"}}`，不打上游、不记成功 spend。
3. **身份**  
   解析 VerificationToken → User → Team → Organization → Project → End User。合并模型白名单与 `max_budget` / TPM / RPM。见 [identity-and-limits.md](identity-and-limits.md)。模型不在允许列表：**401 或 403**（与该路径现网一致），不打上游。
4. **Pre-call 限额**  
   检查 `max_budget`（含 `soft_budget` 只告警）、`max_parallel_requests`、RPM、TPM（本次输入估计 + 输出上限）。超限：**429** + `Retry-After`，不打上游。预扣见 [spend.md](spend.md)。
5. **入站治理**  
   跑 **PROXY_HOOKS**（至少 `max_budget_limiter`、`parallel_request_limiter`）以及 Guardrail / Policy。`block`：按 fail policy 返回，不打上游；`redact`：改写 `messages`/`input` 后继续。
6. **路由**  
   按 `model` 别名取 deployment 池，策略见 [routing-and-cache.md](routing-and-cache.md)。池空或全部冷却：**429/400**，明确 message。
7. **缓存查找**  
   DualCache。命中则跳过上游，响应标 `cache_hit`，头带 `x-litellm-cache-key`，仍按缓存价记 spend。
8. **Adapter**  
   把对外请求转成上游 URL/body。未实现包：**不得**当 OpenAI 乱转发，返回 **`provider_not_implemented`**（400/500，明确 message）。
9. **上游 HTTP/WS**  
   超时、取消（客户端断开必须 cancel）。**一旦发出第一个业务 chunk**（SSE 第一个 token / 第一个非空 JSON 增量）：禁止换 Provider 重试或 fallback；只能把失败记日志。
10. **Adapter 响应**  
    转回对外形状；`model` 字段仍是对外别名。
11. **出站 Guardrail**  
    输出 block/redact。
12. **计价头**  
    写 `x-litellm-response-cost` 及分项。未知价格：见 spend，**不得记 0**。
13. **返回调用方**  
    非流式 JSON 或 `text/event-stream` 以 `data: [DONE]` 结束。
14. **异步**  
    spend 队列 flush → `SpendLogs`；success/failure callbacks；失败计数与 cooldown。

## Fallback 与重试

- **Retry**：同一 deployment，次数默认 `num_retries=2`，受 `timeout`（默认 60s）约束。
- **Fallback**：换模型或换部署。**每一次 fallback 从第 3 步重新做身份与限额**，不得复用第一次候选集（否则会绕过 Team 白名单或把已耗尽的 `max_budget` 打到第二个部署）。
- 流式已出首 chunk：不再 fallback。

## 管理面

管理 CRUD **不走 Router / Adapter**。顺序：鉴权（角色 / `key_type=management`）→ 校验 body → 事务写库 → 失效 Key DualCache → AuditLog → 返回管理 JSON。失败：**400** 校验、**401/403** 权限、**404** 实体不存在。
