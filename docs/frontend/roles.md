# 角色可见性

菜单可见性改善体验；**拒绝发生在 API**。藏按钮不能代替 403。

## 角色名

会话里同时存在旧展示名与 v2 名。

| 原始角色 | 展示 | 写 | 备注 |
|---|---|---|---|
| `proxy_admin` / `Admin` | Admin | 是 | 侧栏全开（受 UI flags 约束的项除外） |
| `proxy_admin_viewer` | Admin Viewer | **否** | 读与 Admin 对齐；`isViewOnly=true`；会话 userRole 会显示成 Admin，以 `isViewOnly` 为准 |
| `org_admin` / `Org Admin` | Org Admin | 本组织 | Users / Organizations 菜单可见；部分 capability |
| `internal_user` / `Internal User` | Internal User | 自己的资源 | 受 `enabled_ui_pages_internal_users` |
| `internal_user_viewer` / `Internal Viewer` | Internal Viewer | **否** | `isViewOnly=true` |
| 团队 `admin` 成员 | （仍是 internal_user） | 本团队 | Agents / Vector Stores 可被 flag 开例外 |

`rolesWithWriteAccess` = Internal User / Admin / `proxy_admin`（不含 Viewer）。

`rolesAllowedToViewWriteScopedPages` = 上面再加 Admin Viewer / `proxy_admin_viewer`（Models、Agents 等配置页可读）。

## 页面矩阵

| 页面 | proxy_admin | proxy_admin_viewer | internal_user | internal_user_viewer |
|---|---|---|---|---|
| Virtual Keys | 读写 | 读；无 Create / Regenerate / Delete / Block | 自己的 Key | 读自己的；无写 CTA |
| Playground | 写 | **侧栏隐藏**；直链 Access Denied | 写（若页面对其可见） | 隐藏 |
| Models + Endpoints | 读写 | 读；无 Add / Delete / Pause / Credentials 写 Tab | 只读角色除外 | 读 |
| Agents / MCP / Guardrails 等 | 按 capability | 读；无 mutating CTA | 受 disable* 与 enabled pages | 读 |
| Usage / Logs | 全部 | 全部（读） | 自己的数据 | 自己的数据 |
| Teams | 是 | 读 | 所在团队 | 读 |
| Projects / Users / Orgs / Budgets / Access Groups | admin | 读（若菜单可见） | 否（Projects 还要 `enable_projects_ui`） | 否 |
| Settings 整组 | admin | 读（admin 组含 viewer） | 否 | 否 |
| Chat 壳 | 可用 | 可用 | 可用 | 可用 |
| 公开 Hub | 匿名可读 | 匿名可读 | 匿名可读 | 匿名可读 |

## view-only 如何藏 CTA

原则：**读对齐 Admin，不写，不产生费用。**

| 位置 | 行为 |
|---|---|
| 侧栏 | `item.key === "llm-playground" && isViewOnly` → 丢掉 Playground |
| `/api-keys` | `isViewOnly` 时不渲染 Create Key |
| Key 详情 | `canModifyKey` 为 false 时无 Regenerate / Block / Delete / Save（Internal Viewer 即使是 owner 也不能改） |
| Models | 无创建、删除、暂停；不挂 Credentials / Pass-through 写 Tab |
| Cost Optimization | 无 Start shadow eval |
| Playground 直链 | 居中「Access Denied」+ 让 proxy admin 开权限；不加载 ChatUI |

Viewer 点被藏按钮的深链（`/api-keys?create=true`）不得弹出创建模态。

## `enabled_ui_pages_internal_users`

Admin Settings → Internal User Page Visibility 写入 `PATCH /update/ui_settings` 的 `enabled_ui_pages_internal_users`。

- `null` / 未设：内部用户看见其角色允许的全部页（「Not set (all pages visible)」）。
- 非空数组：非 admin 的侧栏只留 `page` ∈ 该数组的项。父组若有可见子项则保留。
- 只列出 internal 角色本来能进的页；admin-only（Users、Settings 子页、Tag Management 等）不出现在这个勾选列表里，勾了也不会显示。
- 侧栏 prop 名 `enabledPagesInternalUsers`，API 字段名 `enabled_ui_pages_internal_users`。

Users / Organizations：非 admin 即使在白名单里，也还要角色或 org_admin。

## 其他 UI flags（`GET /get/ui_settings`）

| 字段 | 效果 |
|---|---|
| `enable_projects_ui` | false 隐藏 Projects |
| `disable_agents_for_internal_users` | 非 admin 隐藏 Agents，除非 `allow_agents_for_team_admins` 且该用户是某团队 admin |
| `disable_vector_stores_for_internal_users` | 同上，Vector Stores |

## Capability（侧栏 roles 数组）

`utils/capabilities.ts`：`viewPolicies` / `viewPrompts` / `viewToolPolicies` 等走 `all_admin_roles`；`viewWorkflowRuns` / `viewMemory` / `viewGuardrailUsage` / `viewGlobalSpend` 仅 proxy admin（含 viewer）。Org admin 另有 `viewDeletedTeams`、`viewOrganizationUsage`。
