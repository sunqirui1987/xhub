# 壳层与导航

默认落地 **`/api-keys`（Virtual Keys）**。`/` 在已登录时直接渲染同一套 Virtual Keys 主列（不是空白 HOME）。未登录把当前 URL 存为 return，整页替换到 `/login`。视觉与组件见 [UI](../ui/README.md)。角色裁剪见 [roles](roles.md)。

## 视口

网关模式（ai-gateway）：

```text
┌─────────────┬──────────────────────────────────┐
│ Sidebar     │ Topbar（DashboardHeader）         │
│  Logo+版本  │  左：ViewSwitcher / 页名           │
│  五组导航   │  右：Docs / 主题 / 通知            │
│             ├──────────────────────────────────┤
│             │ 壳层横幅（按条件，可叠多条）        │
│             ├──────────────────────────────────┤
│  用量卡     │ main：当前页，独立纵向滚动         │
│  账户菜单   │                                  │
└─────────────┴──────────────────────────────────┘
```

整页 `h-screen overflow-hidden`。侧栏自滚，主列自滚。窄屏侧栏收成图标或抽屉。

非网关（插件控制面）、`/chat*`、`/model_hub*`、`/connect` 不用五组侧栏，改用全宽 **Navbar**（左 Logo，右用户菜单）。`/login` `/onboarding` `/mcp/oauth/callback` 无壳。

## 侧栏五组

分组标签大写，与 PRD 产品面一致。`HOME_ROUTE = api-keys`。当前路径的父组自动展开。

```text
AI GATEWAY
  Virtual Keys           /api-keys
  Playground             /playground          （rolesWithWriteAccess；isViewOnly 隐藏）
  Models + Endpoints     /models-and-endpoints
  Agentic ▸
    Agents               /agents
    Workflow Runs        /workflows
    Memory               /memory
  MCP Servers            /mcp-servers
  Skills                 /skills              （admin）
  Guardrails             /guardrails
  Policies               /policies
  Tools ▸
    Search Tools         /search-tools
    Vector Stores        /vector-stores
    Tool Policies        /tool-policies
OBSERVABILITY
  Usage                  /usage
  Cost Optimization      /cost-optimization
  Logs                   /logs
  Guardrails Monitor     /guardrails-monitor
ACCESS CONTROL
  Teams                  /teams
  Projects               /projects            （需 enable_projects_ui）
  Internal Users         /users               （admin；org_admin 可见）
  Organizations          /organizations       （admin；org_admin 可见）
  Access Groups          /access-groups       （admin）
  Budgets                /budgets             （admin）
DEVELOPER TOOLS
  API Reference          /api-reference
  AI Hub                 /model-hub-table
  Learning Resources     外链
  Response Cache         /caching             （admin）
  Experimental ▸
    Prompts              /prompts
    API Playground       /transform-request
    Tag Management       /tag-management      （admin）
    Old Usage            /old-usage
SETTINGS                 （整组 all_admin_roles）
  Settings ▸
    Router Settings      /router-settings
    Logging & Alerts     /logging-and-alerts
    Admin Settings       /admin-panel
    Cost Tracking        /cost-tracking
    UI Theme             /ui-theme
```

侧栏数据：

- Logo：`/get_image`（可被 `/get_logo_url` / 主题覆盖）。点 Logo → `/`（即 Keys）。
- 版本：`GET /health/readiness/details` 的版本字段，链到发行说明。
- 折叠按钮：收成图标轨。
- 子组空了（子项全被角色滤掉）则父组不渲染，避免点进不存在的路由。

## 顶栏

`DashboardHeader`，高 56px，只覆盖内容列。

| 侧 | 控件 |
|---|---|
| 左 | ViewSwitcher + 面包屑页名（`getBreadcrumb(pathname)`，与侧栏同一份 menuGroups） |
| 右 | 控制面已选 worker 时 WorkerDropdown（切换会清 cookie 回 `/login?worker=`）· DocsLink · BlogDropdown · 社区按钮（可被「Hide All Prompts」关掉）· ThemeToggle · NotificationsBell |

账户、角色、退出**不在顶栏**，在侧栏底部 `SidebarAccountMenu`：头像、显示名、角色、Tier、Email、User ID、若干本地偏好开关、Logout。Logout 清 token cookie、return URL、worker 选择，跳 `PROXY_LOGOUT_URL`。

## 壳层横幅

排在顶栏和 `main` 之间，来源 `layout.tsx`。有则显示，无则占位为零。

| 横幅 | 条件 | 行为 |
|---|---|---|
| Debug | `GET /health/readiness/details` → `is_detailed_debug` | 性能警告；不可关 |
| No Redis | `show_no_redis_warning` | 多 worker 无 Redis；链到运维说明 |
| Env credential login | admin 且 `show_env_credential_login_warning` | `UI_USERNAME` / `UI_PASSWORD` 或未设密码时的 master key 仍可当共享管理员登录。可 dismiss，写入 localStorage |
| License expiry | `GET` 许可证信息，临近/过期 | warning 可 session dismiss；critical / expired 不可关 |
| User banner | `GET /get/user_banner` 且 `enabled` | Markdown；按 message+severity+revision 签名 dismiss |

Playground 页内另有实验功能 DeprecationBanner，不属于壳。

## view-only

`useAuthorized().isViewOnly` 在原始角色为 `proxy_admin_viewer` / `internal_user_viewer` 时为 true。会话展示角色会把 `proxy_admin_viewer` 显示成 Admin，**必须用 isViewOnly 藏写按钮**，不能只看 userRole 字符串。

壳层效果：

- 侧栏隐藏 Playground（会产生模型费用）。
- Models + Endpoints、Agents、Logs 等配置页仍可见（读与 Admin 对齐）。
- 页内 Create / Save / Delete / Block / Regenerate / Shadow eval Start 由各页自己藏，见 [roles](roles.md)。

## 内部用户可见页

`SidebarProvider` 启动时 `GET /get/ui_settings`，读取：

| 字段 | 作用 |
|---|---|
| `enabled_ui_pages_internal_users` | 非 `null` 时，非 admin 只渲染 `page` 落在该数组里的项（有可见子项的父组仍显示） |
| `enable_projects_ui` | false 则隐藏 Projects |
| `disable_agents_for_internal_users` | 非 admin 隐藏 Agents；`allow_agents_for_team_admins` 可给团队 admin 开例外 |
| `disable_vector_stores_for_internal_users` | 同上，Vector Stores |

Admin 设置入口：`/admin-panel` 的 Internal User Page Visibility。默认 `null` = 内部用户能看见其角色本来能看见的全部页。白名单**不能**把 admin-only 页开放给 internal_user。

## 非侧栏路由

`/login` `/onboarding` `/connect` `/chat` 及其子路由 `/model_hub` `/model_hub_table` `/mcp/oauth/callback` `/`。

`/` 已登录：Virtual Keys。带 `?invitation_id=` 时 layout 整页转到 `/onboarding?...`。带遗留 `?page=` 时按 `legacyPageRoutes` 转到对应路径并保留其余 query。

## 筛选 URL

列表页的搜索/筛选/排序/分页进 query，刷新不丢。实现细节见 [UI 组件](../ui/components.md) 与 [页面模板](../ui/patterns.md)。
