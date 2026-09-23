# XHub vs LiteLLM 1.102.0 对齐报告

**是不是 100% 了：否。**

冻结基线：`docs/_inventory/catalog.json` → LiteLLM **1.102.0**，`unique_http_routes` **779**，`dashboard_pages` **48**，`provider_packages` **136**，外加 `docs/frontend/binding.md` 控制台读写对。

文档覆盖门禁 `python3 docs/_tools/check_coverage.py` 为 PASS（契约/页规格写全）**不等于** 运行时 100% 对齐。Catalog fallback 对 779 条路由非 404 **不记为 `aligned`**，除非 list/CRUD 形状就是 dashboard 实际消费的契约，并且有 Go 测试或 Playwright 证明。

## 总判据

| 轴 | 分母 | aligned | partial | missing | 100%？ |
|---|---:|---:|---:|---:|---|
| HTTP routes | 779 | 105 | 674 | 0 | 否 |
| Dashboard pages (`page.tsx`) | 48 | 8 | 40 | 0 | 否 |
| Provider packages | 136 | 5 | 0 | 131 | 否 |
| binding.md 控制台页 | 42 | 8 | 34 | 0 | 否 |

**结论：不是 100%。** Playwright CRUD 只覆盖密钥 / 团队 / 邀请用户 / 组织 / 预算 / Playground 对话；35 个管理页 crawl 只证明不崩。136 个 provider 里仅 5 个 adapter，其余 `provider_not_implemented`。

## 分类规则

| 状态 | 含义 |
|---|---|
| `aligned` | 专用 handler（或已证明的数据面）+ Go 测试 / Playwright 证明 dashboard 消费的 list/CRUD 形状 |
| `partial` | 路由存在（含 catalog fallback / 空 stub / 原生占位 JSON / 仅 crawl 不崩），但不是 LiteLLM 完整语义或没有创建/列表交互证据 |
| `missing` | LiteLLM 1.102.0 有、XHub 运行时明确未实现（当前主要用于 131 个 provider） |

证据运行（本报告生成时）：

- `python3 docs/_tools/check_coverage.py` → PASS（`missing_paths 0` 等）
- `go test ./...` → PASS
- `cd frontend && npx playwright test` → **20 passed**

`frontend/e2e/helpers.ts` `DASHBOARD_PAGES` 长度 **35**（管理壳 crawl，不含 `/login` `/chat` `/onboarding` 等）。

check_coverage.py 维度：`missing_paths` / `family_path_gaps` / `l1_incomplete` / `page_gaps` / `provider_gaps` / `mechanism_gaps` / `prd_gaps` — 全部 0。这是**文档**完整，不是运行时 100%。

## 1. Dashboard pages — 48 of 48

来源：LiteLLM `ui/litellm-dashboard/src/app/**/page.tsx` 与 XHub `frontend/src/app/**/page.tsx` 均为 48 个文件（vendored）。

| # | Route | page.tsx | Status | Observable |
|---:|---|---|---|---|
| 1 | `/access-groups` | `(dashboard)/access-groups/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 2 | `/admin-panel` | `(dashboard)/admin-panel/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 3 | `/agents` | `(dashboard)/agents/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 4 | `/api-keys` | `(dashboard)/api-keys/page.tsx` | `aligned` | Playwright: Create New Key → Save your Key + `sk-` + alias listed (`keys-playground.spec.ts`) |
| 5 | `/api-reference` | `(dashboard)/api-reference/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 6 | `/budgets` | `(dashboard)/budgets/page.tsx` | `aligned` | Playwright: Create Budget lists budget (`keys-playground.spec.ts`) |
| 7 | `/caching` | `(dashboard)/caching/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 8 | `/cost-optimization` | `(dashboard)/cost-optimization/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 9 | `/cost-tracking` | `(dashboard)/cost-tracking/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 10 | `/guardrails-monitor` | `(dashboard)/guardrails-monitor/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 11 | `/guardrails` | `(dashboard)/guardrails/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 12 | `/logging-and-alerts` | `(dashboard)/logging-and-alerts/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 13 | `/logs` | `(dashboard)/logs/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 14 | `/mcp-servers` | `(dashboard)/mcp-servers/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 15 | `/memory` | `(dashboard)/memory/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 16 | `/model-hub-table` | `(dashboard)/model-hub-table/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 17 | `/models-and-endpoints` | `(dashboard)/models-and-endpoints/page.tsx` | `partial` | Playwright lists `gpt-4o-mini`; Add Model wizard / credentials / health tab not e2e |
| 18 | `/old-usage` | `(dashboard)/old-usage/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 19 | `/organizations` | `(dashboard)/organizations/page.tsx` | `aligned` | Playwright: Create Organization lists org (`keys-playground.spec.ts`) |
| 20 | `/` | `(dashboard)/page.tsx` | `aligned` | Playwright login lands on Virtual Keys; same surface as `/api-keys` create-key e2e |
| 21 | `/playground` | `(dashboard)/playground/page.tsx` | `aligned` | Playwright: model picker + Send chat turn (`keys-playground.spec.ts`) |
| 22 | `/policies` | `(dashboard)/policies/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 23 | `/projects` | `(dashboard)/projects/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 24 | `/prompts` | `(dashboard)/prompts/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 25 | `/router-settings` | `(dashboard)/router-settings/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 26 | `/search-tools` | `(dashboard)/search-tools/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 27 | `/skills` | `(dashboard)/skills/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 28 | `/tag-management` | `(dashboard)/tag-management/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 29 | `/teams` | `(dashboard)/teams/page.tsx` | `aligned` | Playwright: Create Team lists alias (`keys-playground.spec.ts`) |
| 30 | `/tool-policies` | `(dashboard)/tool-policies/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 31 | `/transform-request` | `(dashboard)/transform-request/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 32 | `/ui-theme` | `(dashboard)/ui-theme/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 33 | `/usage` | `(dashboard)/usage/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 34 | `/users` | `(dashboard)/users/page.tsx` | `aligned` | Playwright: admin listed + Invite User → Invitation Link (`keys-playground.spec.ts`) |
| 35 | `/vector-stores` | `(dashboard)/vector-stores/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 36 | `/workflows` | `(dashboard)/workflows/page.tsx` | `partial` | Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 37 | `/chat/api-keys` | `chat/api-keys/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 38 | `/chat/credentials` | `chat/credentials/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 39 | `/chat/integrations` | `chat/integrations/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 40 | `/chat/logs` | `chat/logs/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 41 | `/chat` | `chat/page.tsx` | `partial` | Playwright: chat shell is not admin sidebar (`pages.spec.ts`); no Chat send e2e on `/chat` |
| 42 | `/chat/usage` | `chat/usage/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 43 | `/connect` | `connect/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 44 | `/login` | `login/page.tsx` | `aligned` | Playwright `login.spec.ts` + SSO button (`sso.spec.ts`) |
| 45 | `/mcp/oauth/callback` | `mcp/oauth/callback/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 46 | `/model_hub` | `model_hub/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 47 | `/model_hub_table` | `model_hub_table/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 48 | `/onboarding` | `onboarding/page.tsx` | `partial` | page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |

小计：aligned 8 / partial 40 / missing 0 （共 48）。

## 2. binding.md 控制台读写对

来源：`docs/frontend/binding.md` 主表（不含壳层额外读）。

| # | 页面 | 读 | 写 | Status | Observable |
|---:|---|---|---|---|---|
| 1 | `/api-keys` | `GET /key/list` `GET /key/info` | `POST /key/generate` `update` `delete` `regenerate` `block`。view-only 无写按钮 | `aligned` | /api-keys: Playwright: Create New Key → Save your Key + `sk-` + alias listed (`keys-playground.spec.ts`) |
| 2 | `/playground` | `GET /model/info` `GET /v1/models` | `POST /v1/chat/completions` 等数据面。view-only 不进页、不发 | `aligned` | /playground: Playwright: model picker + Send chat turn (`keys-playground.spec.ts`) |
| 3 | `/models-and-endpoints` | `GET /v2/model/info` `GET /credentials` | `POST /model/new` `update` `delete` `/health/test_connection` | `partial` | /models-and-endpoints: Playwright lists `gpt-4o-mini`; Add Model wizard / credentials / health tab not e2e |
| 4 | `/agents` | `GET /v1/agents` | `POST/PATCH/DELETE /v1/agents` | `partial` | /agents: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 5 | `/workflows` | `GET /v1/workflows/runs` | `PATCH /v1/workflows/runs/{id}` | `partial` | /workflows: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 6 | `/memory` | `GET /v1/memory` | `DELETE /v1/memory/{key}` | `partial` | /memory: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 7 | `/mcp-servers` | `GET /server` | `POST/PUT/DELETE /server` | `partial` | /mcp-servers: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 8 | `/skills` | `GET /v1/skills` | 同前缀写 | `partial` | /skills: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 9 | `/guardrails` | `GET /guardrails/list` | `POST /guardrails` | `partial` | /guardrails: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 10 | `/policies` | `GET /policies/list` | `POST /policies` | `partial` | /policies: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 11 | `/search-tools` | `GET /search_tools/list` | CRUD + test_connection | `partial` | /search-tools: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 12 | `/vector-stores` | `GET /vector_store/list` | `/vector_store/new` | `partial` | /vector-stores: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 13 | `/tool-policies` | `/v1/tool/list` | 覆盖写 | `partial` | /tool-policies: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 14 | `/usage` `/old-usage` | `/global/spend` daily activity | 无 | `partial` | /usage: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction /old-usage: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 15 | `/cost-optimization` | `/auto_router/*` | `shadow_eval/start`（view-only 隐藏） | `partial` | /cost-optimization: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 16 | `/logs` | `/spend/logs` | 无 | `partial` | /logs: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 17 | `/guardrails-monitor` | `/guardrails/usage/*` | 无 | `partial` | /guardrails-monitor: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 18 | `/teams` | `/team/list` | `/team/new` `member_*` | `aligned` | /teams: Playwright: Create Team lists alias (`keys-playground.spec.ts`) |
| 19 | `/projects` | `/project/list` | `/project/new` | `partial` | /projects: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 20 | `/users` | `/user/list` | `/user/new` | `aligned` | /users: Playwright: admin listed + Invite User → Invitation Link (`keys-playground.spec.ts`) |
| 21 | `/organizations` | `/organization/list` | `/organization/new` | `aligned` | /organizations: Playwright: Create Organization lists org (`keys-playground.spec.ts`) |
| 22 | `/access-groups` | `/access_group/list` | `/access_group/new` | `partial` | /access-groups: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 23 | `/budgets` | `/budget/list` | `/budget/new` | `aligned` | /budgets: Playwright: Create Budget lists budget (`keys-playground.spec.ts`) |
| 24 | `/caching` | `/cache/settings` | `POST /cache/settings` `/flushall` | `partial` | /caching: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 25 | `/prompts` | `/prompts/list` | `/prompts` CRUD `test` | `partial` | /prompts: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 26 | `/transform-request` | `/utils/supported_openai_params` | `POST /utils/transform_request` | `partial` | /transform-request: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 27 | `/tag-management` | `/tag/list` | `/tag/new` | `partial` | /tag-management: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 28 | `/router-settings` | `/router/settings` | fallback 更新 | `partial` | /router-settings: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 29 | `/logging-and-alerts` | `/callbacks/list` `/alerting/settings` | 开关 | `partial` | /logging-and-alerts: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 30 | `/admin-panel` | `/get/sso_settings` `/get/allowed_ips` `/get/ui_settings` | `PATCH /update/sso_settings` `PATCH /update/ui_settings`（含 `enabled_ui_pages_internal_users`） | `partial` | /admin-panel: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 31 | `/cost-tracking` | `/cloudzero/settings` | export | `partial` | /cost-tracking: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 32 | `/ui-theme` | `/get/ui_theme_settings` | PATCH `/upload/logo` | `partial` | /ui-theme: Playwright crawl only: page loads, no Dashboard error (`pages.spec.ts`); no list/create interaction |
| 33 | `/chat` | `GET /v1/models` | `POST /v1/chat/completions` | `partial` | /chat: Playwright: chat shell is not admin sidebar (`pages.spec.ts`); no Chat send e2e on `/chat` |
| 34 | `/chat/api-keys` | `GET /key/list` | 无（或有限 rotate） | `partial` | /chat/api-keys: page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 35 | `/chat/logs` | `GET /spend/logs` | 无 | `partial` | /chat/logs: page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 36 | `/chat/usage` | `GET /user/daily/activity` | 无 | `partial` | /chat/usage: page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 37 | `/model_hub` | `GET /public/model_hub` | 无 | `partial` | /model_hub: page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 38 | `/` | 同 `/api-keys`；未登录不打管理读 | 同 `/api-keys` | `aligned` | Playwright login lands on Virtual Keys; same surface as `/api-keys` create-key e2e |
| 39 | `/login` | `GET /.well-known/litellm-ui-config` | `POST /v2/login`；worker 时 `POST /v3/login` + `POST /v3/login/exchange`；SSO `GET /sso/key/generate` → 回调 `?code=` 再 `POST /v3/login/exchange` | `aligned` | /login: Playwright `login.spec.ts` + SSO button (`sso.spec.ts`) |
| 40 | `/onboarding` | `GET /onboarding/get_token?invite_link=` | `POST /onboarding/claim_token` | `partial` | /onboarding: page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 41 | `/connect` | `GET /v1/mcp/server` `GET /authorize/flow` | `POST /authorize/complete`；MCP 用户 OAuth | `partial` | /connect: page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |
| 42 | `/mcp/oauth/callback` | 无（落地页只写 sessionStorage） | 回到发起页后 `POST /v1/mcp/server/oauth/{server_id}/token` | `partial` | /mcp/oauth/callback: page.tsx vendored; no Playwright list/create (not in DASHBOARD_PAGES crawl set) |

小计：aligned 8 / partial 34 / missing 0 （共 42）。

壳层额外读（binding 上表之外）：`GET /health/readiness/details` aligned（health handler + e2e gateway）；`GET /get/ui_settings` aligned；`GET /get/user_banner` / `GET /get_image` / `GET /get_logo_url` **partial**（emptyOK）；`GET /get/ui_theme_settings` partial（dedicated/settings 形状，无主题 e2e）。

## 3. Providers — 136 of 136

来源：`catalog.json` `providers` + `docs/backend-api/runtime/providers.md` + `internal/router.KnownAdapter`。

| # | Package | Status | Observable |
|---:|---|---|---|
| 1 | `a2a` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 2 | `ai21` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 3 | `aiml` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 4 | `aiohttp_openai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 5 | `amazon_nova` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 6 | `anthropic` | `aligned` | `KnownAdapter` + docs `adapter`; dataplane tests (openai/anthropic/gemini/azure/vertex_ai) |
| 7 | `apiserpent` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 8 | `aws_polly` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 9 | `azure` | `aligned` | `KnownAdapter` + docs `adapter`; dataplane tests (openai/anthropic/gemini/azure/vertex_ai) |
| 10 | `azure_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 11 | `base_llm` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 12 | `baseten` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 13 | `bedrock` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 14 | `bedrock_mantle` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 15 | `black_forest_labs` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 16 | `brave` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 17 | `bytez` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 18 | `cerebras` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 19 | `chatgpt` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 20 | `clarifai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 21 | `cloudflare` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 22 | `codestral` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 23 | `cohere` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 24 | `cometapi` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 25 | `compactifai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 26 | `custom_httpx` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 27 | `dashscope` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 28 | `databricks` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 29 | `dataforseo` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 30 | `datarobot` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 31 | `deepgram` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 32 | `deepinfra` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 33 | `deepseek` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 34 | `deprecated_providers` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 35 | `docker_model_runner` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 36 | `duckduckgo` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 37 | `e2b` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 38 | `elevenlabs` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 39 | `empower` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 40 | `exa_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 41 | `fal_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 42 | `fastcrw` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 43 | `featherless_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 44 | `firecrawl` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 45 | `fireworks_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 46 | `friendliai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 47 | `galadriel` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 48 | `gdc` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 49 | `gemini` | `aligned` | `KnownAdapter` + docs `adapter`; dataplane tests (openai/anthropic/gemini/azure/vertex_ai) |
| 50 | `gigachat` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 51 | `github` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 52 | `github_copilot` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 53 | `google_pse` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 54 | `gradient_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 55 | `groq` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 56 | `heroku` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 57 | `hosted_vllm` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 58 | `huggingface` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 59 | `hyperbolic` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 60 | `inception` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 61 | `infinity` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 62 | `jina_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 63 | `lambda_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 64 | `langflow` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 65 | `langgraph` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 66 | `lemonade` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 67 | `linkup` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 68 | `litellm_proxy` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 69 | `llamafile` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 70 | `lm_studio` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 71 | `manus` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 72 | `meta` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 73 | `meta_llama` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 74 | `milvus` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 75 | `minimax` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 76 | `mistral` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 77 | `modelscope` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 78 | `mongodb` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 79 | `moonshot` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 80 | `morph` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 81 | `nebius` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 82 | `nimble` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 83 | `nlp_cloud` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 84 | `novita` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 85 | `nscale` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 86 | `nvidia_nim` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 87 | `nvidia_riva` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 88 | `oci` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 89 | `ollama` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 90 | `oobabooga` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 91 | `openai` | `aligned` | `KnownAdapter` + docs `adapter`; dataplane tests (openai/anthropic/gemini/azure/vertex_ai) |
| 92 | `openai_like` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 93 | `openrouter` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 94 | `opensandbox` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 95 | `ovhcloud` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 96 | `parallel_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 97 | `pass_through` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 98 | `perplexity` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 99 | `petals` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 100 | `pg_vector` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 101 | `predibase` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 102 | `ragflow` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 103 | `recraft` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 104 | `reducto` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 105 | `replicate` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 106 | `runwayml` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 107 | `s3_vectors` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 108 | `sagemaker` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 109 | `sambanova` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 110 | `sap` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 111 | `scaleway` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 112 | `searchapi` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 113 | `searxng` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 114 | `serper` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 115 | `snowflake` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 116 | `soniox` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 117 | `stability` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 118 | `tavily` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 119 | `tencent` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 120 | `tinyfish` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 121 | `together_ai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 122 | `topaz` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 123 | `triton` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 124 | `v0` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 125 | `valkey` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 126 | `vercel_ai_gateway` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 127 | `vertex_ai` | `aligned` | `KnownAdapter` + docs `adapter`; dataplane tests (openai/anthropic/gemini/azure/vertex_ai) |
| 128 | `vllm` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 129 | `volcengine` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 130 | `voyage` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 131 | `wandb` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 132 | `watsonx` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 133 | `xai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 134 | `xinference` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 135 | `you_com` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |
| 136 | `zai` | `missing` | `provider_not_implemented` at runtime (`dataplane.go`); LiteLLM 1.102.0 implements this package |

小计：aligned 5 / partial 0 / missing 131 （共 136）。

## 4. HTTP routes — 779 of 779

来源：`catalog.json` `http_routes`。专用 mux 来自 `internal/server` `HandleFunc`（不含 catch-all `/` catalogFallback）。

| # | Method | Path | Family | Status | Observable |
|---:|---|---|---|---|---|
| 1 | `GET` | `/` | `ui.pages` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 2 | `GET` | `/.well-known/agent-skills/index.json` | `data.skills_tools_memory` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 3 | `GET` | `/.well-known/jwks.json` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 4 | `GET` | `/.well-known/litellm-cli-auth` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 5 | `GET` | `/.well-known/litellm-ui-config` | `(unassigned)` | `aligned` | dedicated `GET /.well-known/litellm-ui-config` + Go test and/or Playwright (dashboard-consumed shape) |
| 6 | `GET` | `/.well-known/oauth-authorization-server` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 7 | `GET` | `/.well-known/oauth-authorization-server/{root_path:path}/v1/mcp/oauth` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 8 | `GET` | `/.well-known/oauth-protected-resource` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 9 | `GET` | `/.well-known/openid-configuration` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 10 | `GET` | `/.well-known/skills/index.json` | `data.skills_tools_memory` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 11 | `POST` | `/a2a/{agent_id}` | `data.a2a` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 12 | `GET` | `/a2a/{agent_id}/.well-known/agent-card.json` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 13 | `GET` | `/a2a/{agent_id}/.well-known/agent.json` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 14 | `POST` | `/a2a/{agent_id}/message/send` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 15 | `GET` | `/access_group/list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 16 | `POST` | `/access_group/new` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 17 | `DELETE` | `/access_group/{access_group}/budget` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 18 | `GET` | `/access_group/{access_group}/budget` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 19 | `PUT` | `/access_group/{access_group}/budget` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 20 | `DELETE` | `/access_group/{access_group}/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 21 | `GET` | `/access_group/{access_group}/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 22 | `PUT` | `/access_group/{access_group}/update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 23 | `GET` | `/access_groups` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 24 | `GET` | `/active/callbacks` | `mgmt.callbacks` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 25 | `GET` | `/adaptive_router/state` | `mgmt.router` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 26 | `POST` | `/add/allowed_ip` | `mgmt.allowed_ips` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 27 | `GET` | `/agent/daily/activity` | `data.agents` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 28 | `GET` | `/alerting/settings` | `mgmt.alerting` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 29 | `POST` | `/api/event_logging/batch` | `data.claude_code` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 30 | `GET` | `/api/plugins` | `data.claude_code` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 31 | `GET` | `/api/plugins/auth-token` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 32 | `POST` | `/apply_guardrail` | `mgmt.guardrails` | `partial` | dedicated `POST /apply_guardrail` (apply_guardrail HTTP exists; not full LiteLLM guardrail pipeline) |
| 33 | `GET` | `/assistants` | `data.assistants_threads` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 34 | `POST` | `/assistants` | `data.assistants_threads` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 35 | `DELETE` | `/assistants/{assistant_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 36 | `POST` | `/audio/speech` | `data.audio` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 37 | `POST` | `/audio/transcriptions` | `data.audio` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 38 | `GET` | `/authorize` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 39 | `POST` | `/authorize/complete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 40 | `GET` | `/authorize/flow` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 41 | `GET` | `/authorize/mcp-session` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 42 | `GET` | `/auto_router/benchmarks` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 43 | `GET` | `/auto_router/classifier/default_prompt` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 44 | `POST` | `/auto_router/classifier/default_prompt` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 45 | `GET` | `/auto_router/session` | `mgmt.router` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 46 | `GET` | `/auto_router/shadow_eval` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 47 | `POST` | `/auto_router/shadow_eval/start` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 48 | `GET` | `/auto_router/shadow_eval/{job_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 49 | `POST` | `/auto_router/shadow_eval/{job_id}/stop` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 50 | `POST` | `/auto_router/test_routing` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 51 | `POST` | `/auto_router/validate_complexity_router_config` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 52 | `GET` | `/batches` | `data.batches` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 53 | `POST` | `/batches` | `data.batches` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 54 | `GET` | `/batches/{batch_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 55 | `POST` | `/batches/{batch_id:path}/cancel` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 56 | `POST` | `/budget/delete` | `mgmt.budgets` | `aligned` | dedicated `POST /budget/delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 57 | `POST` | `/budget/info` | `mgmt.budgets` | `aligned` | dedicated `POST /budget/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 58 | `GET` | `/budget/list` | `mgmt.budgets` | `aligned` | dedicated `GET /budget/list` + Go test and/or Playwright (dashboard-consumed shape) |
| 59 | `POST` | `/budget/new` | `mgmt.budgets` | `aligned` | dedicated `POST /budget/new` + Go test and/or Playwright (dashboard-consumed shape) |
| 60 | `GET` | `/budget/settings` | `(unassigned)` | `aligned` | dedicated `GET /budget/settings` + Go test and/or Playwright (dashboard-consumed shape) |
| 61 | `POST` | `/budget/update` | `mgmt.budgets` | `aligned` | dedicated `POST /budget/update` + Go test and/or Playwright (dashboard-consumed shape) |
| 62 | `GET` | `/budgets` | `ui.pages` | `aligned` | dedicated `GET /budgets` + Go test and/or Playwright (dashboard-consumed shape) |
| 63 | `GET` | `/cache/settings` | `mgmt.cache` | `aligned` | dedicated `GET /cache/settings` + Go test and/or Playwright (dashboard-consumed shape) |
| 64 | `POST` | `/cache/settings` | `mgmt.cache` | `aligned` | dedicated `POST /cache/settings` + Go test and/or Playwright (dashboard-consumed shape) |
| 65 | `POST` | `/cache/settings/test` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 66 | `GET` | `/callback` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 67 | `GET` | `/callbacks/configs` | `mgmt.callbacks` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 68 | `GET` | `/callbacks/list` | `mgmt.callbacks` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 69 | `POST` | `/chat/completions` | `data.chat` | `aligned` | dedicated `POST /chat/completions` + Go test and/or Playwright (dashboard-consumed shape) |
| 70 | `GET` | `/claude-code/marketplace.json` | `data.claude_code` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 71 | `GET` | `/claude-code/plugins` | `data.claude_code` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 72 | `POST` | `/claude-code/plugins` | `data.claude_code` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 73 | `DELETE` | `/claude-code/plugins/{plugin_name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 74 | `GET` | `/claude-code/plugins/{plugin_name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 75 | `PUT` | `/claude-code/plugins/{plugin_name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 76 | `POST` | `/claude-code/plugins/{plugin_name}/disable` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 77 | `POST` | `/claude-code/plugins/{plugin_name}/enable` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 78 | `DELETE` | `/cloudzero/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 79 | `POST` | `/cloudzero/dry-run` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 80 | `POST` | `/cloudzero/export` | `mgmt.cost_export` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 81 | `POST` | `/cloudzero/init` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 82 | `GET` | `/cloudzero/settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 83 | `PUT` | `/cloudzero/settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 84 | `POST` | `/completions` | `data.completions` | `aligned` | dedicated `POST /completions` + Go test and/or Playwright (dashboard-consumed shape) |
| 85 | `POST` | `/compliance/eu-ai-act` | `mgmt.compliance` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 86 | `POST` | `/compliance/gdpr` | `mgmt.compliance` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 87 | `POST` | `/comprehendmedical` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 88 | `POST` | `/comprehendmedical/{operation}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 89 | `GET` | `/config/block_requests_for_models_without_pricing` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 90 | `PATCH` | `/config/block_requests_for_models_without_pricing` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 91 | `POST` | `/config/callback/delete` | `(unassigned)` | `partial` | dedicated `POST /config/callback/delete` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 92 | `GET` | `/config/cost_discount_config` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 93 | `PATCH` | `/config/cost_discount_config` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 94 | `GET` | `/config/cost_margin_config` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 95 | `PATCH` | `/config/cost_margin_config` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 96 | `POST` | `/config/field/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 97 | `GET` | `/config/field/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 98 | `POST` | `/config/field/update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 99 | `GET` | `/config/list` | `mgmt.config` | `aligned` | dedicated `GET /config/list` + Go test and/or Playwright (dashboard-consumed shape) |
| 100 | `DELETE` | `/config/pass_through_endpoint` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 101 | `GET` | `/config/pass_through_endpoint` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 102 | `POST` | `/config/pass_through_endpoint` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 103 | `GET` | `/config/pass_through_endpoint/team/{team_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 104 | `POST` | `/config/pass_through_endpoint/{endpoint_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 105 | `GET` | `/config/pass_through_endpoints/settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 106 | `POST` | `/config/update` | `mgmt.config` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 107 | `GET` | `/config/yaml` | `mgmt.config` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 108 | `DELETE` | `/config_overrides/cyberark` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 109 | `GET` | `/config_overrides/cyberark` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 110 | `POST` | `/config_overrides/cyberark` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 111 | `POST` | `/config_overrides/cyberark/test_connection` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 112 | `DELETE` | `/config_overrides/hashicorp_vault` | `mgmt.config` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 113 | `GET` | `/config_overrides/hashicorp_vault` | `mgmt.config` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 114 | `POST` | `/config_overrides/hashicorp_vault` | `mgmt.config` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 115 | `POST` | `/config_overrides/hashicorp_vault/test_connection` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 116 | `GET` | `/containers` | `data.containers` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 117 | `POST` | `/containers` | `data.containers` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 118 | `DELETE` | `/containers/{container_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 119 | `GET` | `/containers/{container_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 120 | `GET` | `/coordination_redis/settings` | `mgmt.cache` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 121 | `POST` | `/coordination_redis/settings` | `mgmt.cache` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 122 | `POST` | `/coordination_redis/settings/test` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 123 | `POST` | `/cost/estimate` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 124 | `POST` | `/cost/predict-cache` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 125 | `GET` | `/credentials` | `mgmt.credentials` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 126 | `POST` | `/credentials` | `mgmt.credentials` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 127 | `GET` | `/credentials/by_model/{model_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 128 | `GET` | `/credentials/by_name/{credential_name:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 129 | `POST` | `/credentials/migrate-encryption` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 130 | `GET` | `/credentials/migrate-encryption/check` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 131 | `DELETE` | `/credentials/{credential_name:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 132 | `PATCH` | `/credentials/{credential_name:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 133 | `POST` | `/cursor/chat/completions` | `data.chat` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 134 | `GET` | `/cursor/models` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 135 | `GET` | `/cursor/v1/models` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 136 | `POST` | `/customer/block` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 137 | `GET` | `/customer/daily/activity` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 138 | `POST` | `/customer/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 139 | `GET` | `/customer/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 140 | `GET` | `/customer/list` | `mgmt.customers` | `partial` | dedicated `GET /customer/list` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 141 | `POST` | `/customer/new` | `mgmt.customers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 142 | `POST` | `/customer/unblock` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 143 | `POST` | `/customer/update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 144 | `GET` | `/debug/asyncio-tasks` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 145 | `GET` | `/debug/memory/details` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 146 | `POST` | `/debug/memory/gc/configure` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 147 | `GET` | `/debug/memory/summary` | `mgmt.debug` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 148 | `POST` | `/delete` | `mgmt.cache` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 149 | `POST` | `/delete/allowed_ip` | `mgmt.allowed_ips` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 150 | `GET` | `/discover` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 151 | `POST` | `/embeddings` | `data.embeddings` | `aligned` | dedicated `POST /embeddings` + Go test and/or Playwright (dashboard-consumed shape) |
| 152 | `GET` | `/enabled` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 153 | `POST` | `/end_user/block` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 154 | `GET` | `/end_user/daily/activity` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 155 | `POST` | `/end_user/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 156 | `GET` | `/end_user/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 157 | `GET` | `/end_user/list` | `mgmt.customers` | `partial` | dedicated `GET /end_user/list` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 158 | `POST` | `/end_user/new` | `mgmt.customers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 159 | `POST` | `/end_user/unblock` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 160 | `POST` | `/end_user/update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 161 | `POST` | `/engines/{model:path}/chat/completions` | `data.chat` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 162 | `POST` | `/engines/{model:path}/completions` | `data.completions` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 163 | `POST` | `/engines/{model:path}/embeddings` | `data.embeddings` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 164 | `POST` | `/fallback` | `mgmt.router` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 165 | `GET` | `/fallback/login` | `mgmt.sso` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 166 | `DELETE` | `/fallback/{model}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 167 | `GET` | `/fallback/{model}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 168 | `GET` | `/files` | `data.files` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 169 | `POST` | `/files` | `data.files` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 170 | `DELETE` | `/files/{file_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 171 | `GET` | `/files/{file_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 172 | `GET` | `/files/{file_id:path}/content` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 173 | `GET` | `/fine_tuning/jobs` | `data.fine_tuning` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 174 | `POST` | `/fine_tuning/jobs` | `data.fine_tuning` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 175 | `GET` | `/fine_tuning/jobs/{fine_tuning_job_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 176 | `POST` | `/fine_tuning/jobs/{fine_tuning_job_id:path}/cancel` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 177 | `POST` | `/flushall` | `mgmt.cache` | `aligned` | dedicated `POST /flushall` + Go test and/or Playwright (dashboard-consumed shape) |
| 178 | `GET` | `/gateway/daily/activity` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 179 | `GET` | `/get/allowed_ips` | `mgmt.allowed_ips` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 180 | `GET` | `/get/config/callbacks` | `mgmt.callbacks` | `aligned` | dedicated `GET /get/config/callbacks` + Go test and/or Playwright (dashboard-consumed shape) |
| 181 | `GET` | `/get/default_team_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 182 | `GET` | `/get/internal_user_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 183 | `GET` | `/get/mcp_semantic_filter_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 184 | `GET` | `/get/mcp_tool_search_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 185 | `GET` | `/get/sso_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 186 | `GET` | `/get/ui_settings` | `mgmt.ui_settings` | `aligned` | dedicated `GET /get/ui_settings` + Go test and/or Playwright (dashboard-consumed shape) |
| 187 | `GET` | `/get/ui_theme_settings` | `mgmt.ui_settings` | `partial` | dedicated `GET /get/ui_theme_settings` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 188 | `GET` | `/get/user_banner` | `(unassigned)` | `partial` | dedicated `GET /get/user_banner` → emptyOK stub |
| 189 | `GET` | `/get_favicon` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 190 | `GET` | `/get_image` | `(unassigned)` | `partial` | dedicated `GET /get_image` → emptyOK stub |
| 191 | `GET` | `/get_logo_url` | `mgmt.ui_settings` | `partial` | dedicated `GET /get_logo_url` → emptyOK stub |
| 192 | `GET` | `/global/activity` | `mgmt.spend` | `aligned` | dedicated `GET /global/activity` + Go test and/or Playwright (dashboard-consumed shape) |
| 193 | `GET` | `/global/activity/cache_hits` | `(unassigned)` | `partial` | dedicated `GET /global/activity/cache_hits` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 194 | `GET` | `/global/activity/exceptions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 195 | `GET` | `/global/activity/exceptions/deployment` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 196 | `GET` | `/global/activity/model` | `(unassigned)` | `aligned` | dedicated `GET /global/activity/model` + Go test and/or Playwright (dashboard-consumed shape) |
| 197 | `GET` | `/global/all_end_users` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 198 | `GET` | `/global/spend` | `mgmt.spend` | `aligned` | dedicated `GET /global/spend` + Go test and/or Playwright (dashboard-consumed shape) |
| 199 | `GET` | `/global/spend/all_tag_names` | `(unassigned)` | `aligned` | dedicated `GET /global/spend/all_tag_names` + Go test and/or Playwright (dashboard-consumed shape) |
| 200 | `POST` | `/global/spend/end_users` | `(unassigned)` | `partial` | dedicated `POST /global/spend/end_users` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 201 | `GET` | `/global/spend/keys` | `(unassigned)` | `aligned` | dedicated `GET /global/spend/keys` + Go test and/or Playwright (dashboard-consumed shape) |
| 202 | `GET` | `/global/spend/logs` | `(unassigned)` | `aligned` | dedicated `GET /global/spend/logs` + Go test and/or Playwright (dashboard-consumed shape) |
| 203 | `GET` | `/global/spend/models` | `(unassigned)` | `aligned` | dedicated `GET /global/spend/models` + Go test and/or Playwright (dashboard-consumed shape) |
| 204 | `GET` | `/global/spend/provider` | `(unassigned)` | `aligned` | dedicated `GET /global/spend/provider` + Go test and/or Playwright (dashboard-consumed shape) |
| 205 | `POST` | `/global/spend/refresh` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 206 | `GET` | `/global/spend/report` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 207 | `POST` | `/global/spend/reset` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 208 | `GET` | `/global/spend/tags` | `(unassigned)` | `aligned` | dedicated `GET /global/spend/tags` + Go test and/or Playwright (dashboard-consumed shape) |
| 209 | `GET` | `/global/spend/teams` | `(unassigned)` | `aligned` | dedicated `GET /global/spend/teams` + Go test and/or Playwright (dashboard-consumed shape) |
| 210 | `POST` | `/guardrails` | `ui.pages` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 211 | `POST` | `/guardrails/apply_guardrail` | `(unassigned)` | `partial` | dedicated `POST /guardrails/apply_guardrail` (apply_guardrail HTTP exists; not full LiteLLM guardrail pipeline) |
| 212 | `GET` | `/guardrails/list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 213 | `POST` | `/guardrails/register` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 214 | `GET` | `/guardrails/submissions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 215 | `GET` | `/guardrails/submissions/{guardrail_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 216 | `POST` | `/guardrails/submissions/{guardrail_id}/approve` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 217 | `POST` | `/guardrails/submissions/{guardrail_id}/reject` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 218 | `POST` | `/guardrails/test_custom_code` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 219 | `GET` | `/guardrails/ui/add_guardrail_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 220 | `GET` | `/guardrails/ui/category_yaml/{category_name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 221 | `GET` | `/guardrails/ui/major_airlines` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 222 | `GET` | `/guardrails/ui/provider_specific_params` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 223 | `GET` | `/guardrails/usage/detail/{guardrail_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 224 | `GET` | `/guardrails/usage/logs` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 225 | `GET` | `/guardrails/usage/overview` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 226 | `POST` | `/guardrails/validate_blocked_words_file` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 227 | `DELETE` | `/guardrails/{guardrail_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 228 | `GET` | `/guardrails/{guardrail_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 229 | `PATCH` | `/guardrails/{guardrail_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 230 | `PUT` | `/guardrails/{guardrail_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 231 | `GET` | `/guardrails/{guardrail_id}/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 232 | `GET` | `/health` | `mgmt.health` | `aligned` | dedicated `GET /health` + Go test and/or Playwright (dashboard-consumed shape) |
| 233 | `GET` | `/health/backlog` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 234 | `GET` | `/health/drain` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 235 | `GET` | `/health/history` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 236 | `GET` | `/health/latest` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 237 | `GET` | `/health/license` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 238 | `GET` | `/health/liveliness` | `mgmt.health` | `aligned` | dedicated `GET /health/liveliness` + Go test and/or Playwright (dashboard-consumed shape) |
| 239 | `OPTIONS` | `/health/liveliness` | `mgmt.health` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 240 | `GET` | `/health/liveness` | `(unassigned)` | `aligned` | dedicated `GET /health/liveness` + Go test and/or Playwright (dashboard-consumed shape) |
| 241 | `OPTIONS` | `/health/liveness` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 242 | `GET` | `/health/readiness` | `mgmt.health` | `aligned` | dedicated `GET /health/readiness` + Go test and/or Playwright (dashboard-consumed shape) |
| 243 | `OPTIONS` | `/health/readiness` | `mgmt.health` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 244 | `GET` | `/health/readiness/details` | `(unassigned)` | `aligned` | dedicated `GET /health/readiness/details` + Go test and/or Playwright (dashboard-consumed shape) |
| 245 | `GET` | `/health/services` | `(unassigned)` | `partial` | dedicated `GET /health/services` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 246 | `GET` | `/health/shared-status` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 247 | `POST` | `/health/test_connection` | `(unassigned)` | `partial` | dedicated `POST /health/test_connection` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 248 | `POST` | `/images/edits` | `data.images` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 249 | `POST` | `/images/generations` | `data.images` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 250 | `POST` | `/interactions` | `data.interactions` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 251 | `DELETE` | `/interactions/{interaction_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 252 | `GET` | `/interactions/{interaction_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 253 | `POST` | `/interactions/{interaction_id}/cancel` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 254 | `POST` | `/introspect` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 255 | `POST` | `/invitation/delete` | `mgmt.invitations` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 256 | `GET` | `/invitation/info` | `mgmt.invitations` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 257 | `POST` | `/invitation/new` | `mgmt.invitations` | `aligned` | Go test / Playwright dashboard-consumed shape (served via family/data-plane, not catalog stub) |
| 258 | `POST` | `/invitation/update` | `mgmt.invitations` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 259 | `POST` | `/jwt/key/mapping/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 260 | `GET` | `/jwt/key/mapping/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 261 | `GET` | `/jwt/key/mapping/list` | `mgmt.jwt_oidc` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 262 | `POST` | `/jwt/key/mapping/new` | `mgmt.jwt_oidc` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 263 | `POST` | `/jwt/key/mapping/update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 264 | `GET` | `/key/aliases` | `(unassigned)` | `aligned` | dedicated `GET /key/aliases` + Go test and/or Playwright (dashboard-consumed shape) |
| 265 | `POST` | `/key/block` | `(unassigned)` | `aligned` | dedicated `POST /key/block` + Go test and/or Playwright (dashboard-consumed shape) |
| 266 | `POST` | `/key/bulk_update` | `(unassigned)` | `aligned` | dedicated `POST /key/bulk_update` + Go test and/or Playwright (dashboard-consumed shape) |
| 267 | `POST` | `/key/delete` | `mgmt.keys` | `aligned` | dedicated `POST /key/delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 268 | `POST` | `/key/generate` | `mgmt.keys` | `aligned` | dedicated `POST /key/generate` + Go test and/or Playwright (dashboard-consumed shape) |
| 269 | `POST` | `/key/health` | `(unassigned)` | `aligned` | dedicated `POST /key/health` + Go test and/or Playwright (dashboard-consumed shape) |
| 270 | `GET` | `/key/info` | `mgmt.keys` | `aligned` | dedicated `GET /key/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 271 | `GET` | `/key/list` | `mgmt.keys` | `aligned` | dedicated `GET /key/list` + Go test and/or Playwright (dashboard-consumed shape) |
| 272 | `POST` | `/key/regenerate` | `(unassigned)` | `aligned` | dedicated `POST /key/regenerate` + Go test and/or Playwright (dashboard-consumed shape) |
| 273 | `POST` | `/key/service-account/generate` | `(unassigned)` | `aligned` | dedicated `POST /key/service-account/generate` + Go test and/or Playwright (dashboard-consumed shape) |
| 274 | `GET` | `/key/spend/report` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 275 | `POST` | `/key/unblock` | `(unassigned)` | `aligned` | dedicated `POST /key/unblock` + Go test and/or Playwright (dashboard-consumed shape) |
| 276 | `POST` | `/key/update` | `mgmt.keys` | `aligned` | dedicated `POST /key/update` + Go test and/or Playwright (dashboard-consumed shape) |
| 277 | `POST` | `/key/{key:path}/regenerate` | `(unassigned)` | `aligned` | dedicated `POST /key/{key}/regenerate` + Go test and/or Playwright (dashboard-consumed shape) |
| 278 | `POST` | `/key/{key:path}/reset_spend` | `(unassigned)` | `aligned` | dedicated `POST /key/{key}/reset_spend` + Go test and/or Playwright (dashboard-consumed shape) |
| 279 | `POST` | `/lazy/warm/{name}` | `mgmt.debug` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 280 | `GET` | `/litellm/.well-known/litellm-ui-config` | `(unassigned)` | `aligned` | dedicated `GET /litellm/.well-known/litellm-ui-config` + Go test and/or Playwright (dashboard-consumed shape) |
| 281 | `POST` | `/login` | `ui.pages` | `aligned` | dedicated `POST /login` + Go test and/or Playwright (dashboard-consumed shape) |
| 282 | `POST` | `/make_public` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 283 | `GET` | `/memory-usage` | `mgmt.debug` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 284 | `GET` | `/memory-usage-in-mem-cache` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 285 | `GET` | `/memory-usage-in-mem-cache-items` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 286 | `POST` | `/model/block` | `(unassigned)` | `aligned` | dedicated `POST /model/block` + Go test and/or Playwright (dashboard-consumed shape) |
| 287 | `GET` | `/model/cost_map/source` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 288 | `POST` | `/model/delete` | `mgmt.models` | `aligned` | dedicated `POST /model/delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 289 | `GET` | `/model/deprecations` | `data.model_hub` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 290 | `GET` | `/model/info` | `mgmt.models` | `aligned` | dedicated `GET /model/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 291 | `GET` | `/model/metrics` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 292 | `GET` | `/model/metrics/exceptions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 293 | `GET` | `/model/metrics/slow_responses` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 294 | `POST` | `/model/new` | `mgmt.models` | `aligned` | dedicated `POST /model/new` + Go test and/or Playwright (dashboard-consumed shape) |
| 295 | `GET` | `/model/settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 296 | `GET` | `/model/streaming_metrics` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 297 | `POST` | `/model/unblock` | `(unassigned)` | `aligned` | dedicated `POST /model/unblock` + Go test and/or Playwright (dashboard-consumed shape) |
| 298 | `POST` | `/model/update` | `mgmt.models` | `aligned` | dedicated `POST /model/update` + Go test and/or Playwright (dashboard-consumed shape) |
| 299 | `PATCH` | `/model/{model_id}/update` | `(unassigned)` | `aligned` | dedicated `PATCH /model/{model_id}/update` + Go test and/or Playwright (dashboard-consumed shape) |
| 300 | `GET` | `/model_group/info` | `(unassigned)` | `aligned` | dedicated `GET /model_group/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 301 | `POST` | `/model_group/make_public` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 302 | `GET` | `/model_hub` | `ui.pages` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 303 | `POST` | `/model_hub/update_useful_links` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 304 | `GET` | `/model_hub/{facet}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 305 | `GET` | `/models` | `data.model_hub` | `aligned` | dedicated `GET /models` + Go test and/or Playwright (dashboard-consumed shape) |
| 306 | `GET` | `/models/{model_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 307 | `POST` | `/models/{model_name:path}:countTokens` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 308 | `POST` | `/models/{model_name:path}:generateContent` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 309 | `POST` | `/models/{model_name:path}:streamGenerateContent` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 310 | `POST` | `/moderations` | `data.moderations` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 311 | `GET` | `/network/client-ip` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 312 | `POST` | `/ocr` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 313 | `POST` | `/onboarding/claim_token` | `mgmt.onboarding` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 314 | `GET` | `/onboarding/get_token` | `mgmt.onboarding` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 315 | `POST` | `/openai/deployments/{model:path}/chat/completions` | `data.chat` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 316 | `POST` | `/openai/deployments/{model:path}/completions` | `data.completions` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 317 | `POST` | `/openai/deployments/{model:path}/embeddings` | `data.embeddings` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 318 | `POST` | `/openai/deployments/{model:path}/images/edits` | `data.images` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 319 | `POST` | `/openai/deployments/{model:path}/images/generations` | `data.images` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 320 | `WEBSOCKET` | `/openai/v1/realtime` | `data.realtime` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 321 | `POST` | `/openai/v1/realtime/calls` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 322 | `POST` | `/openai/v1/realtime/client_secrets` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 323 | `POST` | `/openai/v1/realtime/transcription_sessions` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 324 | `POST` | `/openai/v1/responses` | `data.responses` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 325 | `POST` | `/openai/v1/responses/compact` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 326 | `POST` | `/openai/v1/responses/input_tokens` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 327 | `DELETE` | `/openai/v1/responses/{response_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 328 | `GET` | `/openai/v1/responses/{response_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 329 | `POST` | `/openai/v1/responses/{response_id}/cancel` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 330 | `GET` | `/openai/v1/responses/{response_id}/input_items` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 331 | `WEBSOCKET` | `/openai/{endpoint:path}` | `data.passthrough` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 332 | `WEBSOCKET` | `/openai_passthrough/{endpoint:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 333 | `GET` | `/openapi-registry` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 334 | `GET` | `/organization/daily/activity` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 335 | `DELETE` | `/organization/delete` | `(unassigned)` | `aligned` | dedicated `DELETE /organization/delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 336 | `GET` | `/organization/info` | `mgmt.organizations` | `aligned` | dedicated `GET /organization/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 337 | `POST` | `/organization/info` | `mgmt.organizations` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 338 | `GET` | `/organization/list` | `mgmt.organizations` | `aligned` | dedicated `GET /organization/list` + Go test and/or Playwright (dashboard-consumed shape) |
| 339 | `POST` | `/organization/member_add` | `(unassigned)` | `aligned` | dedicated `POST /organization/member_add` + Go test and/or Playwright (dashboard-consumed shape) |
| 340 | `DELETE` | `/organization/member_delete` | `(unassigned)` | `aligned` | dedicated `DELETE /organization/member_delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 341 | `PATCH` | `/organization/member_update` | `(unassigned)` | `aligned` | dedicated `PATCH /organization/member_update` + Go test and/or Playwright (dashboard-consumed shape) |
| 342 | `POST` | `/organization/new` | `mgmt.organizations` | `aligned` | dedicated `POST /organization/new` + Go test and/or Playwright (dashboard-consumed shape) |
| 343 | `GET` | `/organization/spend/report` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 344 | `PATCH` | `/organization/update` | `(unassigned)` | `aligned` | dedicated `PATCH /organization/update` + Go test and/or Playwright (dashboard-consumed shape) |
| 345 | `GET` | `/otel-spans` | `mgmt.debug` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 346 | `GET` | `/ping` | `mgmt.cache` | `partial` | dedicated `GET /ping` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 347 | `POST` | `/policies` | `ui.pages` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 348 | `POST` | `/policies/attachments` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 349 | `POST` | `/policies/attachments/estimate-impact` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 350 | `GET` | `/policies/attachments/list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 351 | `DELETE` | `/policies/attachments/{attachment_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 352 | `GET` | `/policies/attachments/{attachment_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 353 | `GET` | `/policies/compare` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 354 | `GET` | `/policies/list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 355 | `DELETE` | `/policies/name/{policy_name}/all-versions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 356 | `GET` | `/policies/name/{policy_name}/versions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 357 | `POST` | `/policies/name/{policy_name}/versions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 358 | `POST` | `/policies/resolve` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 359 | `POST` | `/policies/test-pipeline` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 360 | `GET` | `/policies/usage/overview` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 361 | `DELETE` | `/policies/{policy_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 362 | `GET` | `/policies/{policy_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 363 | `PUT` | `/policies/{policy_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 364 | `GET` | `/policies/{policy_id}/resolved-guardrails` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 365 | `PUT` | `/policies/{policy_id}/status` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 366 | `GET` | `/policy/info/{policy_name}` | `mgmt.policies` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 367 | `GET` | `/policy/list` | `mgmt.policies` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 368 | `GET` | `/policy/templates` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 369 | `POST` | `/policy/templates/enrich` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 370 | `POST` | `/policy/templates/enrich/stream` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 371 | `POST` | `/policy/templates/suggest` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 372 | `POST` | `/policy/templates/test` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 373 | `POST` | `/policy/test` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 374 | `POST` | `/policy/validate` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 375 | `POST` | `/prompts` | `ui.pages` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 376 | `GET` | `/prompts/list` | `mgmt.prompts` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 377 | `POST` | `/prompts/test` | `mgmt.prompts` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 378 | `DELETE` | `/prompts/{prompt_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 379 | `GET` | `/prompts/{prompt_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 380 | `PATCH` | `/prompts/{prompt_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 381 | `PUT` | `/prompts/{prompt_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 382 | `GET` | `/prompts/{prompt_id}/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 383 | `GET` | `/prompts/{prompt_id}/versions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 384 | `GET` | `/provider/budgets` | `mgmt.budgets` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 385 | `GET` | `/public/agent_hub` | `mgmt.public` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 386 | `GET` | `/public/agents/fields` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 387 | `GET` | `/public/autorouter_presets` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 388 | `GET` | `/public/complexity_router/scorer_defaults` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 389 | `GET` | `/public/endpoints` | `mgmt.public` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 390 | `GET` | `/public/litellm_blog_posts` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 391 | `GET` | `/public/litellm_model_cost_map` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 392 | `GET` | `/public/mcp_hub` | `mgmt.public` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 393 | `GET` | `/public/model_hub` | `data.model_hub` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 394 | `GET` | `/public/model_hub/info` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 395 | `GET` | `/public/providers` | `mgmt.public` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 396 | `GET` | `/public/providers/fields` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 397 | `GET` | `/public/skill_hub` | `mgmt.public` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 398 | `POST` | `/queue/chat/completions` | `data.chat` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 399 | `POST` | `/rag/ingest` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 400 | `POST` | `/rag/query` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 401 | `WEBSOCKET` | `/realtime` | `data.realtime` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 402 | `POST` | `/realtime/calls` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 403 | `POST` | `/realtime/client_secrets` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 404 | `POST` | `/realtime/transcription_sessions` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 405 | `GET` | `/redis/info` | `mgmt.cache` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 406 | `POST` | `/register` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 407 | `GET` | `/registry.json` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 408 | `POST` | `/reload/anthropic_beta_headers` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 409 | `POST` | `/reload/model_cost_map` | `mgmt.config` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 410 | `POST` | `/rerank` | `data.rerank` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 411 | `POST` | `/responses` | `data.responses` | `partial` | dedicated `POST /responses` native stub (frozen keys, not real Responses provider) |
| 412 | `WEBSOCKET` | `/responses` | `data.responses` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 413 | `POST` | `/responses/compact` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 414 | `POST` | `/responses/input_tokens` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 415 | `DELETE` | `/responses/{response_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 416 | `GET` | `/responses/{response_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 417 | `POST` | `/responses/{response_id}/cancel` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 418 | `GET` | `/responses/{response_id}/input_items` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 419 | `POST` | `/revoke` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 420 | `GET` | `/router/fields` | `mgmt.router` | `aligned` | dedicated `GET /router/fields` + Go test and/or Playwright (dashboard-consumed shape) |
| 421 | `GET` | `/router/settings` | `mgmt.router` | `aligned` | dedicated `GET /router/settings` + Go test and/or Playwright (dashboard-consumed shape) |
| 422 | `GET` | `/routes` | `mgmt.router` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 423 | `DELETE` | `/schedule/anthropic_beta_headers_reload` | `mgmt.ops_schedules` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 424 | `POST` | `/schedule/anthropic_beta_headers_reload` | `mgmt.ops_schedules` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 425 | `GET` | `/schedule/anthropic_beta_headers_reload/status` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 426 | `DELETE` | `/schedule/model_cost_map_reload` | `mgmt.ops_schedules` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 427 | `POST` | `/schedule/model_cost_map_reload` | `mgmt.ops_schedules` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 428 | `GET` | `/schedule/model_cost_map_reload/status` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 429 | `POST` | `/search` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 430 | `GET` | `/search/tools` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 431 | `POST` | `/search/{search_tool_name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 432 | `POST` | `/search_tools` | `mgmt.search_tools` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 433 | `GET` | `/search_tools/list` | `mgmt.search_tools` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 434 | `POST` | `/search_tools/test_connection` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 435 | `GET` | `/search_tools/ui/available_providers` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 436 | `DELETE` | `/search_tools/{search_tool_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 437 | `GET` | `/search_tools/{search_tool_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 438 | `PUT` | `/search_tools/{search_tool_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 439 | `GET` | `/server` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 440 | `POST` | `/server` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 441 | `PUT` | `/server` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 442 | `GET` | `/server/health` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 443 | `POST` | `/server/import` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 444 | `POST` | `/server/oauth/session` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 445 | `GET` | `/server/oauth/{server_id}/authorize` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 446 | `POST` | `/server/oauth/{server_id}/register` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 447 | `POST` | `/server/oauth/{server_id}/token` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 448 | `POST` | `/server/register` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 449 | `GET` | `/server/submissions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 450 | `DELETE` | `/server/{server_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 451 | `GET` | `/server/{server_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 452 | `PUT` | `/server/{server_id}/approve` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 453 | `DELETE` | `/server/{server_id}/oauth-user-credential` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 454 | `POST` | `/server/{server_id}/oauth-user-credential` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 455 | `GET` | `/server/{server_id}/oauth-user-credential/status` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 456 | `PUT` | `/server/{server_id}/reject` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 457 | `DELETE` | `/server/{server_id}/user-credential` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 458 | `POST` | `/server/{server_id}/user-credential` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 459 | `DELETE` | `/server/{server_id}/user-env-vars` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 460 | `GET` | `/server/{server_id}/user-env-vars` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 461 | `POST` | `/server/{server_id}/user-env-vars` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 462 | `GET` | `/settings` | `mgmt.health` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 463 | `POST` | `/spend/calculate` | `(unassigned)` | `partial` | dedicated `POST /spend/calculate` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 464 | `GET` | `/spend/keys` | `(unassigned)` | `partial` | dedicated `GET /spend/keys` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 465 | `GET` | `/spend/logs` | `mgmt.spend` | `aligned` | dedicated `GET /spend/logs` + Go test and/or Playwright (dashboard-consumed shape) |
| 466 | `GET` | `/spend/logs/session/ui` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 467 | `GET` | `/spend/logs/ui` | `(unassigned)` | `aligned` | dedicated `GET /spend/logs/ui` + Go test and/or Playwright (dashboard-consumed shape) |
| 468 | `GET` | `/spend/logs/ui/{request_id}` | `(unassigned)` | `partial` | dedicated `GET /spend/logs/ui/{request_id}` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 469 | `GET` | `/spend/logs/v2` | `(unassigned)` | `aligned` | dedicated `GET /spend/logs/v2` + Go test and/or Playwright (dashboard-consumed shape) |
| 470 | `GET` | `/spend/tags` | `(unassigned)` | `partial` | dedicated `GET /spend/tags` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 471 | `GET` | `/spend/users` | `(unassigned)` | `partial` | dedicated `GET /spend/users` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 472 | `GET` | `/spend_logs/end_users` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 473 | `GET` | `/spend_logs/users` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 474 | `GET` | `/sso/callback` | `mgmt.sso` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 475 | `POST` | `/sso/cli/complete/{login_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 476 | `GET` | `/sso/cli/poll/{key_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 477 | `POST` | `/sso/cli/start` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 478 | `GET` | `/sso/debug/callback` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 479 | `GET` | `/sso/debug/login` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 480 | `GET` | `/sso/get/ui_settings` | `(unassigned)` | `aligned` | dedicated `GET /sso/get/ui_settings` + Go test and/or Playwright (dashboard-consumed shape) |
| 481 | `GET` | `/sso/key/generate` | `(unassigned)` | `aligned` | dedicated `GET /sso/key/generate` + Go test and/or Playwright (dashboard-consumed shape) |
| 482 | `GET` | `/sso/readiness` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 483 | `POST` | `/sso/saml/callback` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 484 | `GET` | `/sso/saml/login` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 485 | `GET` | `/sso/saml/metadata` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 486 | `GET` | `/tag/daily/activity` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 487 | `GET` | `/tag/dau` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 488 | `POST` | `/tag/delete` | `mgmt.tags` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 489 | `GET` | `/tag/distinct` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 490 | `POST` | `/tag/info` | `mgmt.tags` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 491 | `GET` | `/tag/list` | `mgmt.tags` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 492 | `GET` | `/tag/mau` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 493 | `POST` | `/tag/new` | `mgmt.tags` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 494 | `GET` | `/tag/summary` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 495 | `POST` | `/tag/update` | `mgmt.tags` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 496 | `GET` | `/tag/user-agent/per-user-analytics` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 497 | `GET` | `/tag/wau` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 498 | `GET` | `/team/available` | `(unassigned)` | `aligned` | dedicated `GET /team/available` + Go test and/or Playwright (dashboard-consumed shape) |
| 499 | `POST` | `/team/block` | `(unassigned)` | `aligned` | dedicated `POST /team/block` + Go test and/or Playwright (dashboard-consumed shape) |
| 500 | `POST` | `/team/bulk_member_add` | `(unassigned)` | `aligned` | dedicated `POST /team/bulk_member_add` + Go test and/or Playwright (dashboard-consumed shape) |
| 501 | `GET` | `/team/daily/activity` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 502 | `GET` | `/team/daily/activity/aggregated` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 503 | `POST` | `/team/delete` | `(unassigned)` | `aligned` | dedicated `POST /team/delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 504 | `GET` | `/team/filter/ui` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 505 | `GET` | `/team/info` | `mgmt.teams` | `aligned` | dedicated `GET /team/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 506 | `POST` | `/team/key/bulk_update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 507 | `GET` | `/team/list` | `mgmt.teams` | `aligned` | dedicated `GET /team/list` + Go test and/or Playwright (dashboard-consumed shape) |
| 508 | `POST` | `/team/member_add` | `(unassigned)` | `aligned` | dedicated `POST /team/member_add` + Go test and/or Playwright (dashboard-consumed shape) |
| 509 | `POST` | `/team/member_delete` | `(unassigned)` | `aligned` | dedicated `POST /team/member_delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 510 | `POST` | `/team/member_update` | `(unassigned)` | `aligned` | dedicated `POST /team/member_update` + Go test and/or Playwright (dashboard-consumed shape) |
| 511 | `GET` | `/team/metadata_schema` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 512 | `POST` | `/team/model/add` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 513 | `POST` | `/team/model/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 514 | `POST` | `/team/new` | `mgmt.teams` | `aligned` | dedicated `POST /team/new` + Go test and/or Playwright (dashboard-consumed shape) |
| 515 | `POST` | `/team/permissions_bulk_update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 516 | `GET` | `/team/permissions_list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 517 | `POST` | `/team/permissions_update` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 518 | `GET` | `/team/spend/by_user` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 519 | `GET` | `/team/spend/report` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 520 | `POST` | `/team/unblock` | `(unassigned)` | `aligned` | dedicated `POST /team/unblock` + Go test and/or Playwright (dashboard-consumed shape) |
| 521 | `POST` | `/team/update` | `(unassigned)` | `aligned` | dedicated `POST /team/update` + Go test and/or Playwright (dashboard-consumed shape) |
| 522 | `GET` | `/team/{team_id:path}/callback` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 523 | `POST` | `/team/{team_id:path}/callback` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 524 | `DELETE` | `/team/{team_id:path}/callback/{callback_name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 525 | `PATCH` | `/team/{team_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 526 | `POST` | `/team/{team_id}/disable_logging` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 527 | `POST` | `/team/{team_id}/member/{user_id}/reset_spend` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 528 | `GET` | `/team/{team_id}/members/me` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 529 | `GET` | `/test` | `mgmt.health` | `partial` | dedicated `GET /test` (handler exists; dashboard CRUD / LiteLLM semantics not fully proven) |
| 530 | `POST` | `/test/connection` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 531 | `POST` | `/test/tools/list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 532 | `POST` | `/threads` | `data.assistants_threads` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 533 | `GET` | `/threads/{thread_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 534 | `GET` | `/threads/{thread_id}/messages` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 535 | `POST` | `/threads/{thread_id}/messages` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 536 | `POST` | `/threads/{thread_id}/runs` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 537 | `POST` | `/token` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 538 | `GET` | `/token/generate` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 539 | `GET` | `/tools` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 540 | `POST` | `/tools/call` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 541 | `GET` | `/tools/list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 542 | `GET` | `/toolset` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 543 | `POST` | `/toolset` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 544 | `PUT` | `/toolset` | `mgmt.mcp_servers` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 545 | `DELETE` | `/toolset/{toolset_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 546 | `GET` | `/toolset/{toolset_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 547 | `PATCH` | `/update/default_team_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 548 | `PATCH` | `/update/internal_user_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 549 | `PATCH` | `/update/mcp_semantic_filter_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 550 | `PATCH` | `/update/mcp_tool_search_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 551 | `PATCH` | `/update/sso_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 552 | `PATCH` | `/update/ui_settings` | `mgmt.ui_settings` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 553 | `PATCH` | `/update/ui_theme_settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 554 | `PATCH` | `/update/user_banner` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 555 | `POST` | `/upload/logo` | `mgmt.ui_settings` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 556 | `POST` | `/usage/ai/chat` | `mgmt.spend` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 557 | `GET` | `/user-credentials` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 558 | `GET` | `/user-env-vars/status` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 559 | `GET` | `/user/available_roles` | `(unassigned)` | `aligned` | dedicated `GET /user/available_roles` + Go test and/or Playwright (dashboard-consumed shape) |
| 560 | `POST` | `/user/bulk_update` | `(unassigned)` | `aligned` | dedicated `POST /user/bulk_update` + Go test and/or Playwright (dashboard-consumed shape) |
| 561 | `GET` | `/user/daily/activity` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 562 | `GET` | `/user/daily/activity/aggregated` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 563 | `POST` | `/user/delete` | `mgmt.users` | `aligned` | dedicated `POST /user/delete` + Go test and/or Playwright (dashboard-consumed shape) |
| 564 | `GET` | `/user/filter/ui` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 565 | `GET` | `/user/info` | `mgmt.users` | `aligned` | dedicated `GET /user/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 566 | `GET` | `/user/list` | `mgmt.users` | `aligned` | dedicated `GET /user/list` + Go test and/or Playwright (dashboard-consumed shape) |
| 567 | `POST` | `/user/new` | `mgmt.users` | `aligned` | dedicated `POST /user/new` + Go test and/or Playwright (dashboard-consumed shape) |
| 568 | `GET` | `/user/spend/report` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 569 | `POST` | `/user/update` | `mgmt.users` | `aligned` | dedicated `POST /user/update` + Go test and/or Playwright (dashboard-consumed shape) |
| 570 | `GET` | `/utils/available_routes` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 571 | `POST` | `/utils/dotprompt_json_converter` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 572 | `GET` | `/utils/supported_openai_params` | `mgmt.utils` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 573 | `POST` | `/utils/test_policies_and_guardrails` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 574 | `POST` | `/utils/token_counter` | `mgmt.utils` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 575 | `POST` | `/utils/transform_request` | `mgmt.utils` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 576 | `POST` | `/v1/a2a/discover` | `data.a2a` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 577 | `POST` | `/v1/a2a/{agent_id}/message/send` | `data.a2a` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 578 | `GET` | `/v1/access_group` | `data.access_groups` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 579 | `POST` | `/v1/access_group` | `data.access_groups` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 580 | `DELETE` | `/v1/access_group/{access_group_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 581 | `GET` | `/v1/access_group/{access_group_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 582 | `PUT` | `/v1/access_group/{access_group_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 583 | `GET` | `/v1/agents` | `data.agents` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 584 | `POST` | `/v1/agents` | `data.agents` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 585 | `POST` | `/v1/agents/make_public` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 586 | `DELETE` | `/v1/agents/{agent_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 587 | `GET` | `/v1/agents/{agent_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 588 | `PATCH` | `/v1/agents/{agent_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 589 | `PUT` | `/v1/agents/{agent_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 590 | `POST` | `/v1/agents/{agent_id}/make_public` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 591 | `GET` | `/v1/assistants` | `data.assistants_threads` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 592 | `POST` | `/v1/assistants` | `data.assistants_threads` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 593 | `DELETE` | `/v1/assistants/{assistant_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 594 | `POST` | `/v1/audio/speech` | `data.audio` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 595 | `POST` | `/v1/audio/transcriptions` | `data.audio` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 596 | `GET` | `/v1/batches` | `data.batches` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 597 | `POST` | `/v1/batches` | `data.batches` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 598 | `GET` | `/v1/batches/{batch_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 599 | `POST` | `/v1/batches/{batch_id:path}/cancel` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 600 | `POST` | `/v1/chat/completions` | `data.chat` | `aligned` | dedicated `POST /v1/chat/completions` + Go test and/or Playwright (dashboard-consumed shape) |
| 601 | `POST` | `/v1/completions` | `data.completions` | `aligned` | dedicated `POST /v1/completions` + Go test and/or Playwright (dashboard-consumed shape) |
| 602 | `GET` | `/v1/containers` | `data.containers` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 603 | `POST` | `/v1/containers` | `data.containers` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 604 | `DELETE` | `/v1/containers/{container_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 605 | `GET` | `/v1/containers/{container_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 606 | `POST` | `/v1/embeddings` | `data.embeddings` | `aligned` | dedicated `POST /v1/embeddings` + Go test and/or Playwright (dashboard-consumed shape) |
| 607 | `GET` | `/v1/evals` | `data.evals` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 608 | `POST` | `/v1/evals` | `data.evals` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 609 | `DELETE` | `/v1/evals/{eval_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 610 | `GET` | `/v1/evals/{eval_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 611 | `POST` | `/v1/evals/{eval_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 612 | `POST` | `/v1/evals/{eval_id}/cancel` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 613 | `GET` | `/v1/evals/{eval_id}/runs` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 614 | `POST` | `/v1/evals/{eval_id}/runs` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 615 | `DELETE` | `/v1/evals/{eval_id}/runs/{run_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 616 | `GET` | `/v1/evals/{eval_id}/runs/{run_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 617 | `POST` | `/v1/evals/{eval_id}/runs/{run_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 618 | `GET` | `/v1/files` | `data.files` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 619 | `POST` | `/v1/files` | `data.files` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 620 | `DELETE` | `/v1/files/{file_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 621 | `GET` | `/v1/files/{file_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 622 | `GET` | `/v1/files/{file_id:path}/content` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 623 | `GET` | `/v1/fine_tuning/jobs` | `data.fine_tuning` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 624 | `POST` | `/v1/fine_tuning/jobs` | `data.fine_tuning` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 625 | `GET` | `/v1/fine_tuning/jobs/{fine_tuning_job_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 626 | `POST` | `/v1/fine_tuning/jobs/{fine_tuning_job_id:path}/cancel` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 627 | `POST` | `/v1/images/edits` | `data.images` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 628 | `POST` | `/v1/images/generations` | `data.images` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 629 | `GET` | `/v1/indexes` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 630 | `POST` | `/v1/indexes` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 631 | `GET` | `/v1/mcp/oauth/authorize` | `data.mcp` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 632 | `POST` | `/v1/mcp/oauth/authorize` | `data.mcp` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 633 | `POST` | `/v1/mcp/oauth/token` | `data.mcp` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 634 | `GET` | `/v1/memory` | `data.skills_tools_memory` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 635 | `POST` | `/v1/memory` | `data.skills_tools_memory` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 636 | `DELETE` | `/v1/memory/{key:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 637 | `GET` | `/v1/memory/{key:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 638 | `PUT` | `/v1/memory/{key:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 639 | `POST` | `/v1/messages` | `data.messages` | `aligned` | dedicated `POST /v1/messages` + Go test and/or Playwright (dashboard-consumed shape) |
| 640 | `POST` | `/v1/messages/count_tokens` | `data.messages` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 641 | `GET` | `/v1/model/deprecations` | `data.model_hub` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 642 | `GET` | `/v1/model/info` | `mgmt.models` | `aligned` | dedicated `GET /v1/model/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 643 | `GET` | `/v1/models` | `data.model_hub` | `aligned` | dedicated `GET /v1/models` + Go test and/or Playwright (dashboard-consumed shape) |
| 644 | `GET` | `/v1/models/{model_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 645 | `POST` | `/v1/moderations` | `data.moderations` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 646 | `POST` | `/v1/ocr` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 647 | `POST` | `/v1/rag/ingest` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 648 | `POST` | `/v1/rag/query` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 649 | `WEBSOCKET` | `/v1/realtime` | `data.realtime` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 650 | `POST` | `/v1/realtime/calls` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 651 | `POST` | `/v1/realtime/client_secrets` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 652 | `POST` | `/v1/realtime/transcription_sessions` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 653 | `POST` | `/v1/rerank` | `data.rerank` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 654 | `POST` | `/v1/responses` | `data.responses` | `partial` | dedicated `POST /v1/responses` native stub (frozen keys, not real Responses provider) |
| 655 | `WEBSOCKET` | `/v1/responses` | `data.responses` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 656 | `POST` | `/v1/responses/compact` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 657 | `POST` | `/v1/responses/input_tokens` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 658 | `DELETE` | `/v1/responses/{response_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 659 | `GET` | `/v1/responses/{response_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 660 | `POST` | `/v1/responses/{response_id}/cancel` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 661 | `GET` | `/v1/responses/{response_id}/input_items` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 662 | `POST` | `/v1/search` | `data.search_ocr_rag` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 663 | `GET` | `/v1/search/tools` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 664 | `POST` | `/v1/search/{search_tool_name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 665 | `GET` | `/v1/skills` | `data.skills_tools_memory` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 666 | `POST` | `/v1/skills` | `data.skills_tools_memory` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 667 | `DELETE` | `/v1/skills/{skill_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 668 | `GET` | `/v1/skills/{skill_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 669 | `GET` | `/v1/skills/{skill_id}/archive` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 670 | `POST` | `/v1/threads` | `data.assistants_threads` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 671 | `GET` | `/v1/threads/{thread_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 672 | `GET` | `/v1/threads/{thread_id}/messages` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 673 | `POST` | `/v1/threads/{thread_id}/messages` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 674 | `POST` | `/v1/threads/{thread_id}/runs` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 675 | `GET` | `/v1/tool/list` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 676 | `POST` | `/v1/tool/policy` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 677 | `GET` | `/v1/tool/policy/options` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 678 | `GET` | `/v1/tool/spend` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 679 | `GET` | `/v1/tool/{tool_name:path}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 680 | `GET` | `/v1/tool/{tool_name:path}/detail` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 681 | `GET` | `/v1/tool/{tool_name:path}/logs` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 682 | `DELETE` | `/v1/tool/{tool_name:path}/overrides` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 683 | `GET` | `/v1/vector_store/list` | `data.vector_stores` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 684 | `GET` | `/v1/vector_stores` | `data.vector_stores` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 685 | `POST` | `/v1/vector_stores` | `data.vector_stores` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 686 | `POST` | `/v1/vector_stores/{vector_store_id:path}/search` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 687 | `DELETE` | `/v1/vector_stores/{vector_store_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 688 | `GET` | `/v1/vector_stores/{vector_store_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 689 | `POST` | `/v1/vector_stores/{vector_store_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 690 | `GET` | `/v1/vector_stores/{vector_store_id}/files` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 691 | `POST` | `/v1/vector_stores/{vector_store_id}/files` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 692 | `DELETE` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 693 | `GET` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 694 | `POST` | `/v1/vector_stores/{vector_store_id}/files/{file_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 695 | `GET` | `/v1/vector_stores/{vector_store_id}/files/{file_id}/content` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 696 | `GET` | `/v1/videos` | `data.videos` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 697 | `POST` | `/v1/videos` | `data.videos` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 698 | `POST` | `/v1/videos/characters` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 699 | `GET` | `/v1/videos/characters/{character_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 700 | `POST` | `/v1/videos/edits` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 701 | `POST` | `/v1/videos/extensions` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 702 | `GET` | `/v1/videos/{video_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 703 | `GET` | `/v1/videos/{video_id}/content` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 704 | `POST` | `/v1/videos/{video_id}/remix` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 705 | `GET` | `/v1/workflows/runs` | `data.workflows` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 706 | `POST` | `/v1/workflows/runs` | `data.workflows` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 707 | `GET` | `/v1/workflows/runs/{run_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 708 | `PATCH` | `/v1/workflows/runs/{run_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 709 | `GET` | `/v1/workflows/runs/{run_id}/events` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 710 | `POST` | `/v1/workflows/runs/{run_id}/events` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 711 | `GET` | `/v1/workflows/runs/{run_id}/messages` | `(unassigned)` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 712 | `POST` | `/v1/workflows/runs/{run_id}/messages` | `(unassigned)` | `partial` | catalog fallback → dataPlane when inferenceOp matches; not a dedicated mux route (alias) |
| 713 | `GET` | `/v1beta/agents` | `data.agents` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 714 | `POST` | `/v1beta/agents` | `data.agents` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 715 | `DELETE` | `/v1beta/agents/{name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 716 | `GET` | `/v1beta/agents/{name}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 717 | `GET` | `/v1beta/agents/{name}/versions` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 718 | `POST` | `/v1beta/interactions` | `data.interactions` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 719 | `DELETE` | `/v1beta/interactions/{interaction_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 720 | `GET` | `/v1beta/interactions/{interaction_id}` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 721 | `POST` | `/v1beta/interactions/{interaction_id}/cancel` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 722 | `POST` | `/v1beta/models/{model_name:path}:countTokens` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 723 | `POST` | `/v1beta/models/{model_name:path}:generateContent` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 724 | `POST` | `/v1beta/models/{model_name:path}:streamGenerateContent` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 725 | `GET` | `/v2/guardrails/list` | `mgmt.guardrails` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 726 | `POST` | `/v2/key/info` | `mgmt.keys` | `aligned` | dedicated `POST /v2/key/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 727 | `POST` | `/v2/login` | `mgmt.sso` | `aligned` | dedicated `POST /v2/login` + Go test and/or Playwright (dashboard-consumed shape) |
| 728 | `GET` | `/v2/model/info` | `mgmt.models` | `aligned` | dedicated `GET /v2/model/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 729 | `PATCH` | `/v2/organization/{organization_id}` | `mgmt.organizations` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 730 | `POST` | `/v2/rerank` | `data.rerank` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 731 | `GET` | `/v2/team/list` | `mgmt.teams` | `aligned` | dedicated `GET /v2/team/list` + Go test and/or Playwright (dashboard-consumed shape) |
| 732 | `GET` | `/v2/user/info` | `mgmt.users` | `aligned` | dedicated `GET /v2/user/info` + Go test and/or Playwright (dashboard-consumed shape) |
| 733 | `POST` | `/v3/login` | `mgmt.sso` | `aligned` | dedicated `POST /v3/login` + Go test and/or Playwright (dashboard-consumed shape) |
| 734 | `POST` | `/v3/login/exchange` | `(unassigned)` | `aligned` | dedicated `POST /v3/login/exchange` + Go test and/or Playwright (dashboard-consumed shape) |
| 735 | `DELETE` | `/vantage/delete` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 736 | `POST` | `/vantage/dry-run` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 737 | `POST` | `/vantage/export` | `mgmt.cost_export` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 738 | `POST` | `/vantage/init` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 739 | `GET` | `/vantage/settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 740 | `PUT` | `/vantage/settings` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 741 | `POST` | `/vector_store/delete` | `mgmt.vector_stores_admin` (enterprise) | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 742 | `POST` | `/vector_store/info` | `mgmt.vector_stores_admin` (enterprise) | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 743 | `GET` | `/vector_store/list` | `mgmt.vector_stores_admin` (enterprise) | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 744 | `POST` | `/vector_store/new` | `mgmt.vector_stores_admin` (enterprise) | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 745 | `POST` | `/vector_store/update` | `mgmt.vector_stores_admin` (enterprise) | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 746 | `GET` | `/vector_stores` | `data.vector_stores` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 747 | `POST` | `/vector_stores` | `data.vector_stores` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 748 | `POST` | `/vector_stores/{vector_store_id:path}/search` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 749 | `DELETE` | `/vector_stores/{vector_store_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 750 | `GET` | `/vector_stores/{vector_store_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 751 | `POST` | `/vector_stores/{vector_store_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 752 | `GET` | `/vector_stores/{vector_store_id}/files` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 753 | `POST` | `/vector_stores/{vector_store_id}/files` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 754 | `DELETE` | `/vector_stores/{vector_store_id}/files/{file_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 755 | `GET` | `/vector_stores/{vector_store_id}/files/{file_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 756 | `POST` | `/vector_stores/{vector_store_id}/files/{file_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 757 | `GET` | `/vector_stores/{vector_store_id}/files/{file_id}/content` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 758 | `WEBSOCKET` | `/vertex_ai/live` | `data.realtime` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 759 | `GET` | `/videos` | `data.videos` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 760 | `POST` | `/videos` | `data.videos` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 761 | `POST` | `/videos/characters` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 762 | `GET` | `/videos/characters/{character_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 763 | `POST` | `/videos/edits` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 764 | `POST` | `/videos/extensions` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 765 | `GET` | `/videos/{video_id}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 766 | `GET` | `/videos/{video_id}/content` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 767 | `POST` | `/videos/{video_id}/remix` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 768 | `GET` | `/{mcp_server_name}/authorize` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 769 | `POST` | `/{mcp_server_name}/register` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 770 | `POST` | `/{mcp_server_name}/token` | `(unassigned)` | `partial` | catalog fallback (`catalogFallback` / `catalogListBody`); TestEveryCatalogAPI non-404 — not counted as aligned |
| 771 | `GET` | `/{provider}/v1/batches` | `data.batches` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 772 | `POST` | `/{provider}/v1/batches` | `data.batches` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 773 | `GET` | `/{provider}/v1/batches/{batch_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 774 | `POST` | `/{provider}/v1/batches/{batch_id:path}/cancel` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 775 | `GET` | `/{provider}/v1/files` | `data.files` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 776 | `POST` | `/{provider}/v1/files` | `data.files` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 777 | `DELETE` | `/{provider}/v1/files/{file_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 778 | `GET` | `/{provider}/v1/files/{file_id:path}` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |
| 779 | `GET` | `/{provider}/v1/files/{file_id:path}/content` | `(unassigned)` | `partial` | catalog/family native stub (`writeInferenceNative` / `resourceCRUD`); TestEveryCatalogAPI non-404, not dashboard CRUD |

小计：aligned 105 / partial 674 / missing 0 （共 779）。

### HTTP 按 family 汇总

| Family | enterprise | aligned | partial | missing | total |
|---|---|---:|---:|---:|---:|
| `(unassigned)` | false | 47 | 461 | 0 | 508 |
| `data.a2a` | false | 0 | 3 | 0 | 3 |
| `data.access_groups` | false | 0 | 2 | 0 | 2 |
| `data.agents` | false | 0 | 5 | 0 | 5 |
| `data.assistants_threads` | false | 0 | 6 | 0 | 6 |
| `data.audio` | false | 0 | 4 | 0 | 4 |
| `data.batches` | false | 0 | 6 | 0 | 6 |
| `data.chat` | false | 2 | 4 | 0 | 6 |
| `data.claude_code` | false | 0 | 5 | 0 | 5 |
| `data.completions` | false | 2 | 2 | 0 | 4 |
| `data.containers` | false | 0 | 4 | 0 | 4 |
| `data.embeddings` | false | 2 | 2 | 0 | 4 |
| `data.evals` | false | 0 | 2 | 0 | 2 |
| `data.files` | false | 0 | 6 | 0 | 6 |
| `data.fine_tuning` | false | 0 | 4 | 0 | 4 |
| `data.images` | false | 0 | 6 | 0 | 6 |
| `data.interactions` | false | 0 | 2 | 0 | 2 |
| `data.mcp` | false | 0 | 3 | 0 | 3 |
| `data.messages` | false | 1 | 1 | 0 | 2 |
| `data.model_hub` | false | 2 | 3 | 0 | 5 |
| `data.moderations` | false | 0 | 2 | 0 | 2 |
| `data.passthrough` | false | 0 | 1 | 0 | 1 |
| `data.realtime` | false | 0 | 4 | 0 | 4 |
| `data.rerank` | false | 0 | 3 | 0 | 3 |
| `data.responses` | false | 0 | 5 | 0 | 5 |
| `data.search_ocr_rag` | false | 0 | 10 | 0 | 10 |
| `data.skills_tools_memory` | false | 0 | 6 | 0 | 6 |
| `data.vector_stores` | false | 0 | 5 | 0 | 5 |
| `data.videos` | false | 0 | 4 | 0 | 4 |
| `data.workflows` | false | 0 | 2 | 0 | 2 |
| `mgmt.alerting` | false | 0 | 1 | 0 | 1 |
| `mgmt.allowed_ips` | false | 0 | 3 | 0 | 3 |
| `mgmt.budgets` | false | 5 | 1 | 0 | 6 |
| `mgmt.cache` | false | 3 | 5 | 0 | 8 |
| `mgmt.callbacks` | false | 1 | 3 | 0 | 4 |
| `mgmt.compliance` | false | 0 | 2 | 0 | 2 |
| `mgmt.config` | false | 1 | 6 | 0 | 7 |
| `mgmt.cost_export` | false | 0 | 2 | 0 | 2 |
| `mgmt.credentials` | false | 0 | 2 | 0 | 2 |
| `mgmt.customers` | false | 0 | 4 | 0 | 4 |
| `mgmt.debug` | false | 0 | 4 | 0 | 4 |
| `mgmt.guardrails` | false | 0 | 2 | 0 | 2 |
| `mgmt.health` | false | 3 | 4 | 0 | 7 |
| `mgmt.invitations` | false | 1 | 3 | 0 | 4 |
| `mgmt.jwt_oidc` | false | 0 | 2 | 0 | 2 |
| `mgmt.keys` | false | 6 | 0 | 0 | 6 |
| `mgmt.mcp_servers` | false | 0 | 9 | 0 | 9 |
| `mgmt.models` | false | 6 | 0 | 0 | 6 |
| `mgmt.onboarding` | false | 0 | 2 | 0 | 2 |
| `mgmt.ops_schedules` | false | 0 | 4 | 0 | 4 |
| `mgmt.organizations` | false | 3 | 2 | 0 | 5 |
| `mgmt.policies` | false | 0 | 2 | 0 | 2 |
| `mgmt.prompts` | false | 0 | 2 | 0 | 2 |
| `mgmt.public` | false | 0 | 5 | 0 | 5 |
| `mgmt.router` | false | 2 | 4 | 0 | 6 |
| `mgmt.search_tools` | false | 0 | 2 | 0 | 2 |
| `mgmt.spend` | false | 3 | 1 | 0 | 4 |
| `mgmt.sso` | false | 2 | 2 | 0 | 4 |
| `mgmt.tags` | false | 0 | 5 | 0 | 5 |
| `mgmt.teams` | false | 4 | 0 | 0 | 4 |
| `mgmt.ui_settings` | false | 1 | 4 | 0 | 5 |
| `mgmt.users` | false | 6 | 0 | 0 | 6 |
| `mgmt.utils` | false | 0 | 3 | 0 | 3 |
| `mgmt.vector_stores_admin` | true | 0 | 5 | 0 | 5 |
| `ui.pages` | false | 2 | 5 | 0 | 7 |

## 5. 未完成缺口（下一步）

1. **Add Model 向导**（`/models-and-endpoints`）：目前只有列表 e2e，没有 `POST /model/new` UI 向导。
2. **Projects 创建**：`GET /project/list` 已是数组；UI 创建需先选 Team，无 Playwright。
3. **Agents / MCP / Guardrails / Policies / Skills / Memory / Workflows / Vector stores / Search tools / Prompts / Tags / Access groups** 向导：catalog/KV 或 freezeFamily，crawl 不崩，无创建 e2e。
4. **其余 131 个 provider**：运行时 `provider_not_implemented`（相对 LiteLLM 1.102.0 为 missing）。
5. **Chat 壳** `/chat` 及 `/chat/*`：壳层 e2e 有，对话/密钥子页无。
6. **Onboarding / Connect / MCP OAuth callback / Public model hub**：页存在，无 e2e CRUD。
7. **企业族** `mgmt.projects` / `mgmt.audit` / `mgmt.email` / `mgmt.enterprise_misc` 等：文档 `independent-impl`，仍在 779 分母里，多为 catalog fallback → partial。
8. **数据面原生 stub**：images / audio / videos / moderations / rerank / responses 等有冻结 JSON，不是真实上游。

## 6. 证据索引

| 检查 | 结果 |
|---|---|
| `docs/_inventory/catalog.json` baseline | 779 / 48 / 136 |
| `docs/_tools/check_coverage.py` | PASS |
| `go test ./...` | PASS |
| Playwright | 20 passed（含 35 页 crawl + keys/teams/users/orgs/budgets/playground） |
| `DASHBOARD_PAGES` | 35 条 |
| LiteLLM / XHub `page.tsx` | 各 48 |

