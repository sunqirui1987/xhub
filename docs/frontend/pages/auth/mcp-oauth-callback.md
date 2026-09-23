# MCP OAuth 回调

- Route: `/mcp/oauth/callback`
- Nav: AUTH
- Status: `specified`

## 目的

OAuth 提供方把浏览器送回控制台。本页**无表单、不自己换票**：把 `code` / `state` / 错误写入 sessionStorage，立刻 `replace` 回发起页。发起页上的 `useMcpOAuthFlow` / `useUserMcpOAuthFlow` / `useToolsOAuthFlow` 再 `POST` 换 token。

## 布局

无管理壳、无 Navbar。全屏中性底，居中 `max-w-lg` 卡片。

### 顶栏

无。卡片标题说明 MCP OAuth 已完成，可关闭窗口回到控制台。

### 筛选

无。

### 表

无。

### Tab

无。

### 抽屉

无。

### 模态

无。不弹 SecretOnce、不展示 access token。

### URL

| Query | 作用 |
|---|---|
| `code` | 授权码，写入 storage |
| `state` | CSRF，原样转交 |
| `error` `error_description` | 提供方拒绝（如 `access_denied`）；转交 hook 展示，避免误报「code missing」 |

回跳目标：sessionStorage 里的 return URL（必须同 origin）；否则 `/` 或当前 `/ui` 前缀。`replace` 离开本页，不把 `code` 留在历史里。

写入三套 key，避免 admin / 用户 / tools 流抢结果：admin / user / tools 各一份 result；return URL 一份。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 落地回调 | 本页无 HTTP。写 sessionStorage | 短文案「Authorization complete」，随即跳转 |
| 发起页消费结果 | `POST /v1/mcp/server/oauth/{server_id}/token`（`grant_type=authorization_code`，`code` `code_verifier` `redirect_uri` …） | MCP / Connect 页标记已授权 |

`state` 不匹配由发起 hook 拒绝，本页不做校验 UI。

## 字段

| 字段 | 含义 |
|---|---|
| `code` | 授权码 |
| `state` | 防 CSRF |
| `error` | 提供方错误码 |
| `error_description` | 可读原因 |
| `server_id` | 在发起页的 flow state 里，不在本页 query |

## 交互

无登录墙 UI（提供方跳回）。无 view-only CTA。无创建、无 `sk-`。

| 操作 | 结果 |
|---|---|
| 正常带 `code` | 写 storage → replace 回发起页。用户几乎只看到一闪卡片。 |
| 提供方 `error=` | 同样写 storage 并跳转；发起页展示真实错误，不是「code missing」。 |
| 无 window / 无 params | 不写 storage，留在说明卡。 |
| storage 失败 | 吞掉异常；仍尝试跳转。 |
| return URL 跨 origin / 非法 | 忽略，回默认 `/` 或 `/ui`。 |
| 窗口该关却没关 | 卡片说明可手动关闭，结果已写入。 |
| loading | Suspense 回退「Loading…」。无空表。 |
| forbidden | 不在本页做 403 页。state 失败在发起页。 |

## 验收

从 MCP / Connect 点 Connect → IdP → 本路由 → 回到原页且能换票。`error=access_denied` 时原页能看到拒绝原因。不得 mock 表格。
