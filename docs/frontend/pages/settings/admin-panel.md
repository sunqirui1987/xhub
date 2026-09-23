# 管理员设置

- Route: `/admin-panel`
- Nav: SETTINGS
- Status: `specified`

## 目的

proxy_admin 配置 SSO、登录 IP 白名单、内部用户可见页面、以及 SCIM / Vault / 插件等平台开关。默认内部用户参数（预算、角色、默认团队）不在本页，在 Access Control `/users` 的 Default user settings。本页仅 admin。

## 布局

登录后控制台壳。`p-8`：标题 `Admin Access`，说明去 Internal Users 加其他 admin。其下线型 Tab。

### 顶栏

页标题 + 一句说明。无全局 Save；各 Tab 自己的按钮。

### 筛选

无。SSO / IP / 页面开关是配置，不是资源筛选。

### 表

- SSO Settings：只读详情表（Provider、Client ID/Secret 可揭敏、各 endpoint）。空则 EmptyPlaceholder。
- Security Settings → Allowed IPs 模态内表：IP Address + Delete。
- UI Settings：开关行，不是数据表。Internal User Page Visibility 按导航分组 checkbox。
- SCIM：步骤卡（Tenant URL + 建 token 表单），不是用户表。

### Tab

默认 `sso-settings`：

| Tab | 内容 |
|---|---|
| SSO Settings | 当前 IdP 详情；Add / Edit / Delete SSO；Role mappings / Team mappings |
| Security Settings | 旧 Add SSO（deprecated 警告）、Allowed IPs、UI Access Control；无 SSO 登录 fallback URL `{base}/fallback/login` |
| SCIM | `{base}/scim/v2` + 创建 `allowed_routes=["/scim/*"]` 的管理 token |
| UI Settings | 内部用户能力开关 + Page Visibility + User Banner |
| Logging Settings | spend logs 保留等 general_settings 字段 |
| Hashicorp Vault | 密钥后端 |
| CyberArk Conjur | 密钥后端 |
| Plugins | 插件开关 |

### 抽屉

无。配置都在 Tab 或模态。

### 模态

- Add / Edit / Delete SSO（Google / Microsoft / Generic OIDC / SAML：client_id、secret、tenant、authorization/token/userinfo 或 IdP metadata）。
- Show instructions（保存后回调 URL 说明）。
- Manage Allowed IP Addresses（表 + Add IP + Delete 确认）。Add IP 内层 Dialog，字段 `ip` 必填。
- UI Access Control：`ui_access_mode_type` = all_authenticated_users / restricted_sso_group；后者必填 `restricted_sso_group`，可选 `sso_group_jwt_field`。
- SCIM 创建成功展示 token（一次复制）。

Allowed IPs 与 UI Access Control 需 premium；否则 toast，不打开模态。

### URL

`/admin-panel`。Tab 不写入 URL。

## 交互

进入 SSO Tab 拉 `GET /get/sso_settings`。未配置显示 Add；已配置 Edit/Delete。保存 `PATCH /update/sso_settings`。Allowed IPs：`GET /get/allowed_ips`，空则显示 `All IP Addresses Allowed`；Add `POST /add/allowed_ip`；Delete `POST /delete/allowed_ip`。UI Settings 每个 Switch 立即 `PATCH /update/ui_settings`（`enable_projects_ui` / `enable_chat_ui` 成功后约 1s 整页刷新）。Page Visibility：勾选内部用户可见 page key，空选 = 全部可见（存 `null`）；Save 写 `enabled_ui_pages_internal_users`。Admin-only 页不出现在列表。Role mappings 展示 JWT 角色 → proxy 角色。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 读 SSO | `GET /get/sso_settings` | SSO 详情或空态 |
| 保存 SSO | `PATCH /update/sso_settings` | 关模态、刷新详情 |
| IP 列表 | `GET /get/allowed_ips` | IP 表 |
| 加 IP | `POST /add/allowed_ip` | 新行 |
| 删 IP | `POST /delete/allowed_ip` | 行消失或回退 All Allowed |
| 读 UI 设置 | `GET /get/ui_settings` | 开关与页面列表 |
| 保存 UI 设置 | `PATCH /update/ui_settings` | toast；部分开关刷新页面 |
| SCIM token | `POST /key/generate`（allowed_routes `/scim/*`） | 展示 token 供复制 |
| UI Access Control | 读/写 SSO settings 中 `ui_access_mode` | 关模态 toast |

## 字段

| 字段 | 含义 |
|---|---|
| `google_client_id` / `google_client_secret` | Google SSO |
| `microsoft_client_id` / `microsoft_client_secret` / `microsoft_tenant` | Microsoft |
| `generic_client_id` / `generic_*_endpoint` | 通用 OIDC / Okta |
| `saml_idp_metadata_url` / `saml_idp_metadata_xml` | SAML |
| `role_mappings` / `team_mappings` | JWT → 角色/团队 |
| `ip` | 允许登录的 IP |
| `enabled_ui_pages_internal_users` | 内部用户可见 page key；`null` = 全开 |
| `enable_chat_ui` / `enable_projects_ui` | Chat 壳 / Projects 菜单 |
| `require_auth_for_public_ai_hub` | 公开 Hub 是否要登录 |
| `disable_model_add_for_internal_users` 等 | 内部用户能力开关 |
| `ui_access_mode_type` | all_authenticated_users / restricted_sso_group |
| `restricted_sso_group` / `sso_group_jwt_field` | 限制登录的 SSO 组 |

默认用户预算/角色不在本页字段里。

## 状态

loading：SSO skeleton。empty：SSO EmptyPlaceholder。forbidden：非 admin 无菜单。premium 不足：Allowed IPs / UI Access Control toast。IP 表单空值红字。SSO secret 默认脱敏，点揭开。SCIM token 创建后展示一次明文供复制。窄屏 Tab `flex-wrap`。

## 验收

SSO 读写、Allowed IPs 增删、内部用户页面勾选保存后对 internal_user 侧栏生效；Default user settings 不出现在本页；桌面与窄屏；不得 mock 表格。
