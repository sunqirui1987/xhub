# Chat · 凭证

- Route: `/chat/credentials`
- Nav: CHAT
- Status: `specified`

## 目的

当前用户存在网关的 **MCP OAuth 连接**（不是管理端命名 LLM 凭证 `/credentials`）。无明文 refresh token。从 Integrations 点 Connect 写入；本页可撤销。

## 布局

Chat 壳主区 `py-8 px-8`。

### 顶栏

`App Credentials` + `Your stored OAuth connections; used automatically in chat`。无添加按钮（去 Integrations）。

### 筛选

无。

### 表

列：App（`alias` || `server_name` || `server_id`）、Connected（相对时间）、Status（Does not expire / Expires in Nd / Expired）、Actions（Revoke）。空态：`No connections yet`，引导去 Integrations 点 Connect。Loading：3 行骨架。

### Tab

无。

### 抽屉

无。无 `GET /credentials/by_name/{name}` 详情。

### 模态

Revoke 确认 AlertDialog。无创建模态。

### URL

`/chat/credentials`。

## 交互

进入 `GET /v1/mcp/user-credentials`。Revoke：`DELETE /v1/mcp/server/{server_id}/oauth-user-credential`，成功从缓存列表剔除。失败 toast 保留行。撤销中该行按钮 loading。表不展示 token 明文。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /v1/mcp/user-credentials` | 表 |
| 撤销 | `DELETE /v1/mcp/server/{server_id}/oauth-user-credential` | 行消失 |

不是 `GET /credentials`。

## 字段

| 字段 | 含义 |
|---|---|
| `server_id` | MCP 服务器 |
| `alias` / `server_name` | 显示名 |
| `created_at` / connected | 授权时间 |
| `expires_at` | OAuth 过期；空 = 不超时 |

## 状态

loading：骨架。empty：虚线空态 + 去 Integrations。forbidden：未进 Chat 壳。撤销失败行还在。窄屏表横向滚。

## 验收

只列当前用户 MCP OAuth；撤销后聊天不再带该 server；无凭证明文；桌面与窄屏。
