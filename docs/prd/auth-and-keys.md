# 鉴权与虚拟 Key

## 头解析顺序（必须按序，命中即停）

1. **`x-litellm-api-key`**  
   若出现，优先于 `Authorization`（透传/浏览器场景）。值仍是 `sk-…` 或 master key。
2. **`Authorization: Bearer`**  
   虚拟 Key 或 master key。
3. **`Authorization: Basic`**
4. **JWT / OIDC**  
   `/.well-known/jwks.json`；声明映射 `/jwt/key/mapping/*`。
5. **SSO session cookie**  
   控制台登录后。Session 不得把上游供应商密钥带到浏览器。
6. 都没有 → **401**，OpenAI 信封 `invalid_api_key` 一类，不打上游。

校验对象是库表 **VerificationToken**：用密钥哈希比对主键 `token`（明文不入库）。还要查 `expires`、`blocked`、`allowed_routes`、IP allowlist。过期或 `blocked=true` → **401**。IP 不在名单 → **403**。

## `key_type`

| 值 | 能调 |
|---|---|
| `llm_api` | 数据面 |
| `management` | 管理面 |
| `read_only` | info/list 只读 |
| `default` | 默认 allowed_routes（通常含数据面） |

`llm_api` 调 `/key/generate` → **403**。`management` 调 `/v1/chat/completions` → **401/403**，除非配置显式允许。

## 生成、存储、展示一次

- `POST /key/generate`（管理面）body 冻结字段：`key_alias`、`models`、`max_budget`、`tpm_limit`、`rpm_limit`、`duration`、`team_id`、`user_id`、`organization_id`、`project_id`、`key_type`、`guardrails`、`object_permission` 等。
- 响应里的 `key` 是明文 `sk-…`，**只这一次**。之后 `GET /key/list` / `GET /key/info` 只给 `key_name` 前缀，不得回完整明文。
- 库：VerificationToken.`token` = 哈希。控制台 Create Key 成功模态（SecretOnce）关闭后无法再看明文；轮换 `POST /key/regenerate` 再给一次新明文。
- view-only 角色不渲染 Create / Regenerate。

失败：无管理权限 **401/403**；非法 `duration` **400**；`max_budget` 空字符串视为 `null`。
