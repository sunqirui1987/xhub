# 鉴权决策

按以下顺序解析调用方（与 `user_api_key_auth` 等价）：

1. **`x-litellm-api-key` 头**（透传场景优先于 `Authorization`）
2. **`Authorization: Bearer`** 虚拟 Key 或 master key
3. **`Authorization: Basic`**
4. **JWT / OIDC**（`/.well-known/jwks.json`，`/jwt/key/mapping/*`）
5. **SSO session cookie**（控制台登录后）
6. 都没有 → 401

校验：哈希比对 VerificationToken；`expires`、`blocked`、`allowed_routes`、IP allowlist。  
master key：可调管理面；默认不可调数据面。  
`key_type`：`llm_api` | `management` | `read_only` | `default`。
