# 组件

| 组件 | 用法 |
|---|---|
| **Sidebar** | 五组导航，可折叠；选中项高亮。分组标签**大写**：`AI GATEWAY` / `OBSERVABILITY` / `ACCESS CONTROL` / `DEVELOPER TOOLS` / `SETTINGS`。嵌套项（Agentic / Tools / Experimental / Settings）点组名展开，不自己当路由。Logo + 版本在头部；底部用量卡（admin）+ 账户菜单。 |
| **Topbar** | 网关壳：左 ViewSwitcher + 当前页标题面包屑；右文档、主题、通知；控制面可有 Worker 切换。账户与退出在侧栏底部，不在顶栏。Chat / 公开 Hub / `/connect` 用独立 Navbar（左 Logo，右用户菜单）。 |
| **PageHeader** | 图标 + 标题 + 说明；主按钮在控件行左侧（Create Key 等）。view-only 不传 `primaryAction`。 |
| **FilterBar / DataTableToolbar** | 搜索、筛选抽屉、刷新。筛选项、排序、分页写入 **URL query**，刷新与分享不丢。 |
| **DataTable** | 列可排序、可调整宽度；行点击进详情（常写 `?key=` 等 id）；行末 `⋯` 菜单。服务端分页与 list API 的 `page` / `page_size` 一致。 |
| **Pagination** | 与 list API 的 page / cursor 一致；页码进 URL。 |
| **Drawer / Tabs** | 筛选用抽屉。详情分组：Overview / Members / Budget 等 Tab。 |
| **Modal** | 创建、确认删除、一次性展示 secret。 |
| **FormField** | label = 字段语义，`name` = API 字段。数字预算用 number。 |
| **StatusBadge** | active / blocked / expired / healthy / unhealthy。 |
| **SecretOnce** | **仅**虚拟 Key 创建 / 轮换成功。完整 `sk-` + Copy；文案写明关闭后无法再查看。关闭后列表只有 `key_name` 前缀。不得用在登录、开通、连接、OAuth 回调、设置页。 |
| **EmptyState** | 短句 + 主按钮（例如还没有 Key）。view-only 有空态、无创建按钮。 |
| **ErrorBanner** | API `error.message`，可关闭。 |
| **ShellBanner** | 顶栏下方全宽警告（调试、无 Redis、环境凭据登录、许可证、运营横幅）。见 [壳层](../frontend/shell.md)。 |
| **ChatBubble** | 仅 `/chat` 与 Playground 输出区；管理页不用气泡做表。 |

组件库可自选（shadcn 等），但上述角色不能缺。

## Sidebar 五组（默认树）

分组标签渲染为大写；面包屑用 Title Case。

```text
AI GATEWAY
  Virtual Keys          → /api-keys          （默认落地）
  Playground            → /playground        （view-only 隐藏）
  Models + Endpoints    → /models-and-endpoints
  Agentic ▸
    Agents / Workflow Runs / Memory
  MCP Servers           → /mcp-servers
  Skills                → /skills            （admin）
  Guardrails            → /guardrails
  Policies              → /policies
  Tools ▸
    Search Tools / Vector Stores / Tool Policies
OBSERVABILITY
  Usage / Cost Optimization / Logs / Guardrails Monitor
ACCESS CONTROL
  Teams / Projects / Internal Users / Organizations / Access Groups / Budgets
DEVELOPER TOOLS
  API Reference / AI Hub / Learning Resources（外链）/ Response Cache
  Experimental ▸ Prompts / API Playground / Tag Management / Old Usage
SETTINGS                （整组 admin）
  Settings ▸ Router Settings / Logging & Alerts / Admin Settings / Cost Tracking / UI Theme
```

`SETTINGS` 整组 `roles: all_admin_roles`。internal_user 另受 `enabled_ui_pages_internal_users` 白名单约束，见 [roles](../frontend/roles.md)。

## Topbar 分区

| 区 | 内容 |
|---|---|
| 左 | ViewSwitcher（网关 / 插件面）+ 分隔 + 当前页标题（由侧栏配置 `getBreadcrumb` 推导） |
| 右 | 控制面 Worker 下拉（若已选 worker）· Docs · Blog · 社区按钮（可关）· 主题 · 通知铃 |
| 不在顶栏 | Logo（侧栏头）、角色、退出（侧栏账户菜单） |

## SecretOnce

创建或 Regenerate 成功后打开模态「Save your Key」：

1. 等宽展示完整 `sk-…`。
2. Copy Virtual Key；复制成功有 toast。
3. 明确：**关闭后无法再通过控制台查看明文**；丢失只能再生成一把。
4. 关闭后表格该行只显示 `key_alias` 与 token 前缀 / 哈希。
5. 明文不写入 localStorage / sessionStorage。

## FilterBar 与 URL

列表页用 query 持久化（实现可用 nuqs 等，行为必须如下）：

| Query | 含义 |
|---|---|
| 搜索 | 如 `key_search` |
| 筛选 | 如 `filter_team` `filter_org` `filter_user` `filter_key_id`（加前缀，避免与创建深链 `team_id` / `key_alias` 冲突） |
| 排序 | `sort_by` `sort_order` |
| 分页 | `page` `page_size` |
| 详情 | 如 `key=` 打开该行详情，返回列表清掉该参数、保留筛选 |

刷新、浏览器后退、把 URL 发给别人，应看到同一筛选结果。
