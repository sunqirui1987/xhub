# 内部用户

- Route: `/users`
- Nav: ACCESS CONTROL
- Status: `specified`

## 目的

内部用户账号与全局代理角色。Proxy admin 可创建、批量改、重置密码、删除；其他人只看列表与自己可见范围。

## 布局

**顶栏** 左主按钮组（仅 proxy admin，列表 loading 时骨架）：Create User、Bulk Create、Select Users（按下后变 Cancel Selection）。选择模式出现 Bulk Edit (N selected)，N=0 时禁用。无选择时不出现 Bulk Edit。非 admin 无这组按钮，直接表。

**筛选** 表工具条搜索（防抖，打 `GET /user/list?search=`）+ Filter Drawer：User ID、SSO ID、Role（`GET /user/available_roles` 的 ui_label）、Team。改筛选/排序/分页清空行选择。默认排序 `created_at` desc，页大小 25。

**表** 选择模式最左多选列。列：User ID（等宽，点击进详情 Overview）、Email、Status（Active；`metadata.scim_active===false` 为 Inactive，tooltip 说明 SCIM 停用且虚拟 Key 被拦）、Global Proxy Role、User Alias、Spend (USD)、Budget (USD)（空为 Unlimited）、SSO ID（列头 Info：非 SSO 用户为 null）、Virtual Keys（N Keys / No Keys 徽章）、Created At、Updated At。行末 ⋯：Edit user、Reset password、Copy user ID、Delete user（危险）。

**Tab** proxy admin 页级 Users / Default User Settings。Users 是表；Default User Settings 是默认角色/预算/模型表单。非 admin 无这组 Tab，只有表。详情 UserInfoView：Overview（spend、团队、Key 摘要）/ Details（可编辑字段）；从 ⋯ Edit 进入时 `initialTab=details` 且 startInEditMode。

**抽屉** 无 Sheet。详情整页。Default User Settings 是 Tab 内容不是抽屉。Filter Drawer 只承载筛选。

**模态** Create User。Bulk Create。Bulk Edit（选择模式）。Delete User 确认（Email、User ID、Role、Total Spend）。Reset password 先 toast「Generating password reset link...」，成功后 OnboardingModal 展示邀请链接（这是重置链，不是密钥明文）。详情内加团队 Dialog、删用户确认。

**URL** `?user=<user_id>` 详情（push）；Back 清除。页级 Tab、筛选、选择模式不进 URL。

## 交互

点 User ID → 详情 Overview。点 ⋯ Edit user → 详情 Details 编辑。点 Reset password → 邀请链接模态，用户用该链接设新密码。点 Copy user ID → 剪贴板。点 Delete user → 确认；成功后乐观从当前页去掉该行。点 Select Users → 多选；Bulk Edit 提交 `POST /user/bulk_update` 后退出选择模式并失效列表。点 Default User Settings → 保存默认值，影响之后 `POST /user/new`，不改已存在用户。Create User 成功后新行出现，不展示 sk-。

empty：有筛选「No users found」——Try adjusting your search or filters；无筛选同样组件但应理解为还没有内部用户（与筛选空态共用结构，靠是否有 filter 区分）。forbidden：非 proxy admin 藏 Create / Bulk / Select / 行内 Edit Delete Reset；Copy 与只读详情仍可用。越权 list 用说明，不把全站用户画成空。SCIM Inactive 不是 403。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /user/list` | 表 |
| 角色字典 | `GET /user/available_roles` | Role 列与筛选 |
| 创建 | `POST /user/new` | 新行 |
| 更新 | `POST /user/update` | 详情保存 |
| 删除 | `POST /user/delete` | 行消失 |
| 批量 | `POST /user/bulk_update` | 多行更新，退出选择 |
| 重置密码 | `POST` 邀请创建 | 打开邀请链接模态 |
| 详情 | `GET /v2/user/info` | Overview / Details |

## 字段

| 字段 | 含义 |
|---|---|
| `user_id` | id |
| `user_email` | 邮箱 |
| `user_role` | proxy_admin / internal_user 等 |
| `user_alias` | 显示名 |
| `max_budget` | 预算 |
| `spend` | 已花费 |
| `sso_user_id` | SSO 侧 id |
| `key_count` | 虚拟 Key 数 |
| `scim_active` | 元数据；false 则 Inactive |

## 状态

loading：按钮骨架 + 表骨架。empty / forbidden 见交互。校验失败贴在 Create / Details / Default Settings 字段旁。删除与 bulk 进行中锁确认。超时锁定原 payload。选择模式切换时清空 selection，避免把筛选前的勾选提交到 bulk。

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。Reset password 只展示邀请链接，不得出现密钥明文。SCIM Inactive 必须带说明 tooltip。
