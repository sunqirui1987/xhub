# 页面与契约绑定

禁止另起 `/api/v1/console/...`。控制台只打下表（及页面规格里增补的同行路径）。view-only 仍发**读**请求；写 CTA 隐藏，误打写接口必须 403。

壳层额外读（所有已登录管理页）：

| 用途 | Method Path |
|---|---|
| 版本、调试/Redis/环境凭据横幅 | `GET /health/readiness/details` |
| 侧栏裁剪 | `GET /get/ui_settings`（`enabled_ui_pages_internal_users` 等） |
| 运营横幅 | `GET /get/user_banner` |
| Logo / 主题 | `GET /get_image` `GET /get_logo_url` `GET /get/ui_theme_settings` |
| 许可证横幅 | 许可证 info GET |

未登录：`GET /.well-known/litellm-ui-config`（登录页是否 SSO、是否禁用 Admin UI）。

| 页面 | 读 | 写 |
|---|---|---|
| `/api-keys` | `GET /key/list` `GET /key/info` | `POST /key/generate` `update` `delete` `regenerate` `block`。view-only 无写按钮 |
| `/playground` | `GET /model/info` `GET /v1/models` | `POST /v1/chat/completions` 等数据面。view-only 不进页、不发 |
| `/models-and-endpoints` | `GET /v2/model/info` `GET /credentials` | `POST /model/new` `update` `delete` `/health/test_connection` |
| `/agents` | `GET /v1/agents` | `POST/PATCH/DELETE /v1/agents` |
| `/workflows` | `GET /v1/workflows/runs` | `PATCH /v1/workflows/runs/{id}` |
| `/memory` | `GET /v1/memory` | `DELETE /v1/memory/{key}` |
| `/mcp-servers` | `GET /server` | `POST/PUT/DELETE /server` |
| `/skills` | `GET /v1/skills` | 同前缀写 |
| `/guardrails` | `GET /guardrails/list` | `POST /guardrails` |
| `/policies` | `GET /policies/list` | `POST /policies` |
| `/search-tools` | `GET /search_tools/list` | CRUD + test_connection |
| `/vector-stores` | `GET /vector_store/list` | `/vector_store/new` |
| `/tool-policies` | `/v1/tool/list` | 覆盖写 |
| `/usage` `/old-usage` | `/global/spend` daily activity | 无 |
| `/cost-optimization` | `/auto_router/*` | `shadow_eval/start`（view-only 隐藏） |
| `/logs` | `/spend/logs` | 无 |
| `/guardrails-monitor` | `/guardrails/usage/*` | 无 |
| `/teams` | `/team/list` | `/team/new` `member_*` |
| `/projects` | `/project/list` | `/project/new` |
| `/users` | `/user/list` | `/user/new` |
| `/organizations` | `/organization/list` | `/organization/new` |
| `/access-groups` | `/access_group/list` | `/access_group/new` |
| `/budgets` | `/budget/list` | `/budget/new` |
| `/caching` | `/cache/settings` | `POST /cache/settings` `/flushall` |
| `/prompts` | `/prompts/list` | `/prompts` CRUD `test` |
| `/transform-request` | `/utils/supported_openai_params` | `POST /utils/transform_request` |
| `/tag-management` | `/tag/list` | `/tag/new` |
| `/router-settings` | `/router/settings` | fallback 更新 |
| `/logging-and-alerts` | `/callbacks/list` `/alerting/settings` | 开关 |
| `/admin-panel` | `/get/sso_settings` `/get/allowed_ips` `/get/ui_settings` | `PATCH /update/sso_settings` `PATCH /update/ui_settings`（含 `enabled_ui_pages_internal_users`） |
| `/cost-tracking` | `/cloudzero/settings` | export |
| `/ui-theme` | `/get/ui_theme_settings` | PATCH `/upload/logo` |
| `/chat` | `GET /v1/models` | `POST /v1/chat/completions` |
| `/chat/api-keys` | `GET /key/list` | 无（或有限 rotate） |
| `/chat/logs` | `GET /spend/logs` | 无 |
| `/chat/usage` | `GET /user/daily/activity` | 无 |
| `/model_hub` | `GET /public/model_hub` | 无 |
| `/` | 同 `/api-keys`；未登录不打管理读 | 同 `/api-keys` |
| `/login` | `GET /.well-known/litellm-ui-config` | `POST /v2/login`；worker 时 `POST /v3/login` + `POST /v3/login/exchange`；SSO `GET /sso/key/generate` → 回调 `?code=` 再 `POST /v3/login/exchange` |
| `/onboarding` | `GET /onboarding/get_token?invite_link=` | `POST /onboarding/claim_token` |
| `/connect` | `GET /v1/mcp/server` `GET /authorize/flow` | `POST /authorize/complete`；MCP 用户 OAuth |
| `/mcp/oauth/callback` | 无（落地页只写 sessionStorage） | 回到发起页后 `POST /v1/mcp/server/oauth/{server_id}/token` |
