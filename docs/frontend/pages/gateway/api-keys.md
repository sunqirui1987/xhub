# 虚拟密钥

- Route: `/api-keys`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

默认首页。创建、筛选、轮换、拉黑虚拟 Key。

## 布局

- **顶栏 CTA**：`PageHeader` 标题 Virtual Keys，副标题说明网关鉴权。右侧 primaryAction 为 `+ Create New Key`（`CreateKey`）。`isViewOnly` 时不渲染该按钮。
- **筛选**：工具栏搜索框「Search by key alias or ID…」（防抖后打 `GET /key/list?search=`，substring matching）。刷新按钮。Filters 抽屉：Team（`SearchSelect`）、Organization、User ID、Key ID。服务端排序：Key、Key ID、Created At、Updated At、Spend / Budget。分页 `page` / `page_size`（默认 50，最大 100）。
- **表**：列 Key（alias + `key_name` 前缀 + Active/Blocked/Expired 徽章）、Key ID、Team、Organization、User、Created At、Created By、Updated At、Last Active、Expires、Spend / Budget、Budget Reset、Models、Rate Limits（TPM/RPM）。点 alias / Key ID 进入详情。
- **抽屉/模态**：
  - Create New Key 对话框：owned_by（you / service_account / another_user）、key_alias、models、key_type（AI APIs / Management / Full Access）、预算与限流等。提交成功后同一模态用 `CreatedKeyDisplay` 展示明文 `sk-…`。
  - 详情页 `KeyInfoView`：返回 Back to Keys；Regenerate Key；更多菜单 Block/Unblock Key、Reset Spend、Delete Key。Tab：Overview、Savings、Auto-Router Usage（条件）、Settings（Edit）。
  - Regenerate Key 模态：成功后再展示一次明文，标题 Save it now, you will not see it again。
  - Delete Key / Block Key 确认模态。
- **URL query**：
  - 创建预填：`?create=true` 自动打开创建框；`owned_by`（you|service_account|another_user）、`team_id`、`key_alias`、`models`（逗号分隔）、`key_type`（default|llm_api|management）。筛选 query 带 `filter_` 前缀，避免劫持这些预填参数。
  - 详情：`?key=<token>`。
  - 表状态：`key_search`、`sort_by`、`sort_order`、`page`、`page_size`、`filter_team`、`filter_org`、`filter_user`、`filter_key_id`。

## 交互

- 点 `+ Create New Key` → 打开创建对话框；提交 `POST /key/generate`（服务账号走 `POST /key/service-account/generate`）→ 模态展示明文 `sk-` 密钥（只此一次，关闭后无法再看）→ 表刷新出现新行。
- `?create=true` 且带预填参数 → 进页即打开创建框并填入 owned_by / team_id / key_alias / models / key_type。
- 点行 alias 或 Key ID → `?key=` 切到详情。详情里 Settings → Edit 提交 `POST /key/update` → 关闭编辑并刷新。Regenerate Key → `POST /key/{id}/regenerate` → 再展示一次明文 `sk-`。Block/Unblock → `POST /key/block` 或 `/key/unblock`。Reset Spend → `POST /key/{id}/reset_spend`。Delete Key → `POST /key/delete` → 返回列表，行消失。
- **view-only**：顶栏不渲染 Create；详情 `canModifyKey` 为 false 时隐藏 Regenerate 与更多菜单（Block/Delete/Reset Spend）以及 Settings 的 Edit。
- **loading**：列表 `Loading keys...`；详情未就绪 `Loading key...`。
- **empty**：列表 `No keys found`。
- **forbidden**：未授权走 `LoadingScreen` / 鉴权门；团队成员无 `/key/generate` 权限时创建错误被简化为联系管理员开通权限。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /key/list` | 填表 |
| 详情 | `GET /key/info` | 详情页 |
| 创建 | `POST /key/generate` | 模态展示明文 key |
| 服务账号创建 | `POST /key/service-account/generate` | 同上 |
| 更新 | `POST /key/update` | 关闭抽屉并刷新 |
| 删除 | `POST /key/delete` | 行消失 |
| 轮换 | `POST /key/{id}/regenerate` | 再展示一次明文 |
| 拉黑 | `POST /key/block` | 状态变为 blocked |
| 解禁 | `POST /key/unblock` | 状态变为 Active |
| 重置花费 | `POST /key/{id}/reset_spend` | spend 归零 |

## 字段

| 字段 | 含义 |
|---|---|
| `key_alias` | 显示名 |
| `token` | 哈希/前缀，非明文 |
| `team_id` | 所属团队 |
| `org_id` | 所属组织 |
| `user_id` | 所属用户 |
| `models` | 允许模型 |
| `spend` | 已花费 USD |
| `max_budget` | 预算 |
| `expires` | 过期 |
| `last_active` | 最近使用（仅新用量回填） |
| `key_type` | llm_api/management/read_only/default |
| `blocked` | 是否拉黑 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
