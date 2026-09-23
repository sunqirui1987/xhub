# Chat · 集成

- Route: `/chat/integrations`
- Nav: CHAT
- Status: `specified`

## 目的

终端用户连接 MCP 应用，供 `/chat` 发送时带 tools。不是管理员 Logging Callbacks，无 webhook 编辑。可选 `?connect_flow=` 走网关密封 connect flow（不信任 URL 里的业务上下文）。

## 布局

Chat 壳主区。默认 `MCPAppsPanel` 卡片列表。

### 顶栏

搜索 MCP 应用。无「Add callback」。Connect flow 进行中时顶部 `ConnectFlowBanner`（状态、失败重试）。

### 筛选

搜索框按 server 名。Tab：All / Connected。

### 表

不是管理表。卡片/列表：logo 或色块头像、名称、工具数量、OAuth Connect 徽章或开关。点行进详情（工具列表）。`connectMode` 时不支持的 auth 显示 `Not supported on this connection`。

### Tab

`all` / `connected`。另：若 URL 有 `connect_flow` 且 flow `unscoped`，仍渲染 Apps 列表供勾选。

### 抽屉

无。详情是面板内 drill-in（返回箭头），不是管理抽屉。

### 模态

OAuth 走浏览器授权（`useUserMcpOAuthFlow`），不是表单模态。无管理员 callback 配置 Dialog。

### URL

`/chat/integrations`。`?connect_flow=` 拉 flow；`?mcpOauthReturn=` 落地后立刻 `router.replace` 删掉该参数。

## 交互

进入 `GET /v1/mcp/server`（`connectedAppView` 视 connectMode）。OAuth 服务器再查 credential status。Connect：authorization_code 开 OAuth；成功加入 `selectedMCPServers`（ChatShellContext，供对话 tools）。Toggle 开：先 `listMCPTools`，失败 warning 不勾选。Toggle 关：从选中列表移除。OAuth 成功自动把该 server 名加入选中。本页不读写 `/callbacks/list`。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 MCP | `GET /v1/mcp/server` | 卡片 |
| 工具数 / 试连 | list MCP tools | 数量或详情 |
| OAuth | MCP OAuth authorize + token | Connected；凭证出现在 `/chat/credentials` |
| Connect flow | `GET` connect-flow（`connect_flow` handle） | Banner 状态 |

不是 `GET /callbacks/list` / `GET /callbacks/configs`。

## 字段

| 字段 | 含义 |
|---|---|
| `server_id` / `server_name` / `alias` | 应用 |
| `auth_type` | none / oauth 等 |
| `connected_app_reachable` | connectMode 过滤 |
| `selectedMCPServers` | 当前对话将携带的 tools |
| `connect_flow` | 密封连接句柄 |

## 状态

loading：卡片骨架。empty：无 MCP。OAuth 中徽章 `Connecting…`。connect flow 失败 Banner + Retry。`mcpOauthReturn` 闪过后 URL 干净。窄屏列表单列。

## 验收

能搜/连 MCP 并在 `/chat` 发送时带 tools；无 Slack webhook 编辑；connect_flow 不把业务参数写进 URL 信任面；桌面与窄屏。
