# 控制台根

- Route: `/`
- Nav: 壳
- Status: `specified`

## 目的

登录后进入控制台。已登录时 `/` 直接渲染 Virtual Keys（与 `/api-keys` 同一主列）。`HOME_ROUTE` 为 `api-keys`。未登录不进壳。

## 布局

已登录走网关壳（侧栏五组 + 顶栏 + 横幅 + 主列），主列即列表模板。未登录无此布局。

### 顶栏

`DashboardHeader`：左 ViewSwitcher + 标题 Virtual Keys；右 Docs / 主题 / 通知。Logo 在侧栏头，点 Logo 仍回 `/`。

### 筛选

主列 Toolbar：按 alias / Key ID 搜索；Filters 抽屉（Team / Organization / User ID / Key ID）。Apply 后写入 `key_search` `filter_team` `filter_org` `filter_user` `filter_key_id` `sort_by` `sort_order` `page` `page_size`。

### 表

Virtual Keys：`key_alias`、token 前缀、team、org、user、models、spend、expires、last_active。行点击打开详情。

### Tab

列表无 Tab。详情（`?key=`）用 KeyInfo 的 Overview 等 Tab。

### 抽屉

筛选抽屉。无右侧业务抽屉；详情替换主列。

### 模态

Create Key（view-only 不渲染）。提交成功叠 SecretOnce：完整 `sk-` + Copy，关闭后列表只留前缀。删除 / Block / Regenerate 在详情页头。

### URL

| Query | 作用 |
|---|---|
| （无） | 已登录 → Keys 列表 |
| `invitation_id` | layout 整页 `replace` 到 `/onboarding?...`，不渲染壳 |
| `page=` | 遗留深链，按 `legacyPageRoutes` 转到新路径，其余 query 保留 |
| `create=true` | 打开创建模态；可带 `owned_by` `team_id` `key_alias` `models` `key_type` 预填 |
| `key=` | 打开该 Key 详情 |
| `filter_*` / `key_search` / `sort_*` / `page` / `page_size` | 列表状态，刷新不丢 |
| `login=success` | 登录回流，仍落 Keys |

未登录：把当前 URL 存为 return，`window.location.replace` 到登录（带 `redirect_to`）。登录后再读 return；仅允许同 origin。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 打开已登录 `/` | `GET /health/readiness/details` `GET /get/ui_settings` `GET /key/list` | 壳 + Keys 表；侧栏版本徽章 |
| 未登录打开 `/` | 不打管理读；跳 `/login` | 登录页 |
| 邀请深链 | 不打 Keys | 去 `/onboarding` |
| 创建 Key | `POST /key/generate` | SecretOnce 模态 |
| 退出（侧栏） | 清 cookie，跳 logout URL | 登录页 |

## 字段

| 字段 | 含义 |
|---|---|
| `redirect_to` | 未登录时保存的回跳；默认落地 `/api-keys` / `/` |
| `session` / `token` cookie | 无则不进壳 |
| `key` | 详情 token id |
| `enabled_ui_pages_internal_users` | 裁侧栏，见 [roles](../../roles.md) |

## 交互

| 操作 | 结果 |
|---|---|
| 已登录打开 `/` | LoadingScreen → 壳 + Keys。不先闪登录。 |
| 未登录 | 立刻 replace 登录，不渲染空表。 |
| 点侧栏 Virtual Keys | `/api-keys`，同一主列。 |
| 点 Logo | `/`。 |
| 改筛选 / 排序 / 页码 | 写 URL，表按 `GET /key/list` 刷新。 |
| 点行 | `?key=` 进详情；Back 去掉 `key`、保留筛选。 |
| 点 Create Key | 开创建模态。view-only **没有此按钮**；`?create=true` 也不得打开。 |
| 创建成功 | SecretOnce 展示一次 `sk-`。关闭后无法再看明文。 |
| Admin Viewer | 能进 `/` 与 Keys；无 Create / Regenerate / Delete / Block；侧栏无 Playground。 |
| 无 Key | 「No keys found」；有写权限才配创建按钮。 |
| 无权限 list | 错误 Banner / forbidden，不显示假 0 行。 |
| 鉴权 loading | 全屏 LoadingScreen。 |
| 壳横幅 | 按 [shell](../../shell.md) 条件出现在顶栏下。 |

## 验收

桌面与窄屏：未登录→登录；已登录→Keys；邀请 query→onboarding；view-only 无写按钮。不得 mock 表格。
