# 登录

- Route: `/login`
- Nav: AUTH
- Status: `specified`

## 目的

三种方式进入控制台，必须都能用：

1. **用户名 + 密码**（内部用户 / 管理员账号）
2. **Master key**：默认提示用户名为 `admin`，密码为网关 **master key**（`MASTER_KEY`；若配置了 `UI_USERNAME` / `UI_PASSWORD` 则用那对。未设 `UI_PASSWORD` 时 master key 仍可作为共享管理员密码）
3. **SSO**（已配置时「Login with SSO」；`auto_redirect_to_sso` 时进页即跳 IdP）

成功写入 session cookie，跳 return URL 或 `/`（Virtual Keys）。无侧栏。

## 布局

居中全屏（`min-h-screen` 中性底），一张 `max-w-lg` 卡片。无管理壳。

### 顶栏

无。无 Logo 导航、无五组侧栏、无 DashboardHeader。卡片顶是产品名与标题 Login。

### 筛选

无。

### 表

无。

### Tab

无。三种方式叠在同一张表单里，不是 Tab。

### 抽屉

无。

### 模态

无。Admin UI 被关时整页换成警告卡片，不是模态。

### URL

| Query | 作用 |
|---|---|
| `redirect_to` / 已存 return cookie | 登录成功后同 origin 回跳 |
| `code` | 跨 origin SSO 回流的一次性码；校验形态后 `POST /v3/login/exchange`，从地址栏删掉 `code`，再 `replace` `/?login=success` |
| `worker` | 控制面选 worker；会清旧 token 并显示 Worker 下拉；提交走 `/v3/login` |
| （无，已有未过期 token） | `replace` return 或 `/` |

`GET /.well-known/litellm-ui-config` 决定：`admin_ui_disabled`、`auto_redirect_to_sso`、`sso_configured`、`hide_default_credentials_hint`、`is_control_plane`。

卡片自上而下：

1. 产品名、Login、说明。
2. 默认凭据提示（可关）：Username 为 `admin`，Password 为已配置的 master key（`MASTER_KEY`），不是字面量 `master key`。
3. 错误 Alert（登录失败 `error.message`）。
4. 控制面且有 worker 列表时：Worker 下拉。
5. Username、Password（密码框可显隐）。
6. 主按钮 Login。
7. Login with SSO：未配置则禁用 + tooltip；已配置则可点。
8. 已配置 SSO 且未自动跳转时：可关闭的 SSO 说明（不再自动跳 IdP，除非环境打开自动跳转）。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 拉 UI 配置 | `GET /.well-known/litellm-ui-config` | 决定表单 / SSO / 禁用 |
| 用户名密码登录 | `POST /v2/login` body `{username,password}` | 存 token cookie，跳 return 或服务端 `redirect_url`（通常 Keys） |
| 控制面 + 已选 worker | `POST /v3/login` 再 `POST /v3/login/exchange` | 跳 `/?login=success`，请求打到该 worker |
| Master key | 同上；username=`admin`（或 `UI_USERNAME`），password=master key（或 `UI_PASSWORD`） | 同密码登录，角色为 proxy admin |
| 点 Login with SSO | `GET {base}/sso/key/generate?return_to=` | 浏览器去 IdP；回 `/sso/callback` 或带 `?code=` |
| SSO `?code=` | `POST /v3/login/exchange` `{code}` | 写 cookie，去 `/` |
| 自动 SSO | `GET /sso/key/generate?redirect_to=` | 不画表单 |
| Admin UI 禁用 | 无写 | 警告卡，说明需打开 Admin UI |

契约族还有 `POST /login`、SAML `GET /sso/saml/login` + `POST /sso/saml/callback`；本页按钮走 `/v2/login` 与 `/sso/key/generate`。

## 字段

| 字段 | 含义 |
|---|---|
| `username` | 登录名。默认提示 `admin` |
| `password` | 用户密码 **或** master key |
| `sso_configured` | 是否启用 SSO 按钮 |
| `auto_redirect_to_sso` | 进页是否直接去 IdP |
| `admin_ui_disabled` | 整页禁用 |
| `code` | SSO / v3 一次性换票码 |
| `worker` | 控制面 worker id |
| `redirect_to` | 登录后回跳 |

## 交互

本页无 view-only：尚未建立会话。Viewer 也走这张表，登录后由壳藏 CTA。

| 操作 | 结果 |
|---|---|
| 配置 loading | 全屏 LoadingScreen，不闪空卡。 |
| 空用户名/密码提交 | 字段错误：Please enter your username / password；不发请求。 |
| 点 Login | 按钮变 Logging in… + spinner，禁用输入。成功跳转；失败卡片顶红 Alert，可改后重试。超时不换用户名密码重放。 |
| 用 master key | 与密码同一按钮、同一 `POST /v2/login`。成功进 Admin。 |
| SSO 未配置 | SSO 按钮 disabled，hover 要求先配置 SSO。 |
| SSO 已配置 | 点 Login with SSO → 跳 `/sso/key/generate`。可选 worker 时先切 base。 |
| `auto_redirect_to_sso` | 不渲染表单，直接 push IdP。 |
| 已登录且 JWT 未过期 | `replace` return 或 `/`，不停留。 |
| `admin_ui_disabled` | 无表单。警告：管理员关闭了 Admin UI。 |
| 错密 / 401 | Alert 展示 `error.message`；不进壳。 |
| 无「空列表」 | 本页不是资源列表。禁用 UI 是 forbidden，不是 empty。 |
| 控制面选错 worker 导致登录失败 | 回滚 worker base，留在本页。 |

不要在本页做 SecretOnce，也不展示 `sk-`。Session token 写入 HttpOnly/cookie，界面不显示明文密钥。

## 验收

桌面与窄屏走通：用户名密码、master key（`admin` + `MASTER_KEY`）、SSO 按钮、`?code=` 换票、UI 禁用卡。不得 mock 表格。
