# 连接引导

- Route: `/connect`
- Nav: AUTH
- Status: `specified`

## 目的

已登录用户把 **MCP 应用**接到网关：浏览可连的 MCP Server、做用户 OAuth、在带 `connect_flow` 的密封流程里 Finish / Cancel，把授权交回发起方。不是 SDK `base_url` 向导；那条路径在 Virtual Keys 的 SecretOnce 与 API Reference。

必须已登录：`useAuthorized` 未决或未授权时 layout 渲染 `null` 并跳登录。

## 布局

独立壳：顶 **Navbar**（左 Logo，右账户），**无五组侧栏**。主区 `max-w-5xl` 居中。

### 顶栏

`Navbar`：Logo、版本、Docs / 主题 / 用户菜单。无 ViewSwitcher 面包屑。

### 筛选

MCP 列表顶：搜索框（按 server 名过滤，**仅本地**，不写 URL）。无 Team/Org 筛选抽屉。

### 表

不是 DataTable。卡片/列表：server 名、工具数、OAuth 状态、Connect。`connectMode` 下不可用的 server 标 Not supported on this connection。

### Tab

列表 **All / Connected**。点一张进该 server 详情（工具列表），顶有返回。

### 抽屉

无。详情替换列表，不是右侧抽屉。

### 模态

无创建 Key 模态。OAuth 走浏览器跳转，回 `/mcp/oauth/callback`。流程结束靠页顶 **ConnectFlowBanner** 的 Finish connecting / Cancel（POST 表单），不是 Modal。

### URL

| Query | 作用 |
|---|---|
| （无 `connect_flow`） | 只渲染 MCPAppsPanel：浏览 / 勾选 / Connect |
| `connect_flow` | `GET /authorize/flow?flow=`。页顶 Banner；`state=unscoped` 时下方仍是列表（`connectMode`） |
| `mcpOauthReturn` | 落地后立刻从 URL 删掉（replace），避免把 OAuth 上下文留在地址栏 |

Banner 文案随 `flow.state`：`unscoped` / `interactive` / 已 connected / `stale`。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 MCP | `GET /v1/mcp/server`（`connected_app_view` 在 connectMode） | 填列表 |
| 工具数 | 列表 MCP tools GET | 卡片上的 count |
| OAuth 是否已授 | 用户 OAuth credential status GET | Connected 标记；自动勾选 |
| 点 Connect | 用户 MCP OAuth 授权 URL → 浏览器 | 回回调页再回本页 |
| 读密封流程 | `GET /authorize/flow?flow=` | Banner 状态 |
| Finish connecting | `POST /authorize/complete`（hidden `flow`；可选 `delivery=manual`） | 回 client_origin |
| Cancel | 同上 `decision=deny` | 回 client，不授码 |

## 字段

| 字段 | 含义 |
|---|---|
| `connect_flow` / `flow` | 密封流程 handle（cookie + query） |
| `state` | `unscoped` `interactive` `m2m` `stale` |
| `client_origin` | 发起连接的应用 |
| `server_id` `server_name` | 流程绑定的 MCP |
| `connected` | 该 server 用户 OAuth 是否完成 |
| `selectedServers` | 当前勾选的 server 名 |

## 交互

需要写权限才能 Finish；view-only 可看列表。Finish / Connect 是 mutating：Viewer 应看到只读列表、无 Finish / Connect（若仍暴露，API 403，界面不得假装已授权）。

| 操作 | 结果 |
|---|---|
| 鉴权 loading / 未登录 | layout `null`，转登录。无空 MCP 表。 |
| 无 `connect_flow`、列表 loading | 列表骨架，不闪「没有服务器」再跳数据。 |
| 零台 MCP | 空列表文案；不是 forbidden。 |
| 点 Connect | 按钮 Connecting…；跳 IdP。失败 toast / error，可再点。 |
| OAuth 回来 | 回调页写 sessionStorage 后 replace 回本页；credential status 变已连。 |
| 流程 `stale` / flow GET 失败 | Banner：The connection cannot continue。可 Cancel 回 client。无 Finish。 |
| `unscoped` | 先在下方勾选并授权，再 Finish connecting。 |
| `interactive` 且未 connected | Banner 要求先授权该 server；可自动拉起 OAuth。 |
| 已 connected | Finish connecting 把授权交回 client。loopback client 可勾「remote or SSH」走手工 delivery。 |
| 搜索 | 只过滤当前列表，不改 query。 |
| 无 SecretOnce | 不展示 `sk-`。OAuth token 不在本页明文常驻。 |

## 验收

桌面与窄屏：无 flow 浏览 MCP；有 flow 出现 Banner 并能 Finish/Cancel；OAuth 往返后状态更新。不得 mock 表格。
