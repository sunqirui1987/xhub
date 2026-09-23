# Vector Stores

- Route: `/vector-stores`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

管理端向量库登记。独立实现，不复制企业源码。

## 布局

- **顶栏 CTA**：标题 Vector Store Management。右侧刷新 + Last Refreshed。可写时 Manage Tab 内 `+ Add Vector Store`。
- **筛选**：无列表搜索。
- **表/卡片/Tab**：
  - Tab：Create Vector Store（proxy admin 且非 view-only）、Manage Vector Stores、Test Vector Store；proxy admin 另有 Indexes。
  - Manage 表列 Vector Store ID、Name、Description、Files、Provider、Created At、Updated At。行 View / Edit / Delete。
  - 详情 Tab：Details、Test Vector Store（Search 试探）。Create Tab 为登记向导。
- **抽屉/模态**：CreateVectorStore 模态（与 Create Tab 同源）；Delete 确认。
- **URL query**：无。选中 id 为组件 state。

## 交互

- Create / Add → `POST /vector_store/new` 新行。点 ID View → Details；Edit 保存 `POST /vector_store/update`。Delete → `POST /vector_store/delete`。Test Tab / 详情 Test → `POST /v1/vector_stores/{id}/search` 结果列表。
- **view-only**：隐藏 Create Tab 与 `+ Add Vector Store`（`isProxyAdminRole && !isViewOnly` 才可写）。Manage / Test 仍可读。
- **loading**：表 loading。
- **empty**：`No vector stores` / Connect a vector store to enable retrieval-augmented generation.
- **forbidden**：非 proxy admin 无 Create/Indexes 写入口。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /vector_store/list` | 表 |
| 创建 | `POST /vector_store/new` | 新行 |
| 更新 | `POST /vector_store/update` | 保存 |
| 删除 | `POST /vector_store/delete` | 移除 |
| 搜索试探 | `POST /v1/vector_stores/{id}/search` | 结果列表 |

## 字段

| 字段 | 含义 |
|---|---|
| `vector_store_name` | 名称 |
| `vector_store_id` | id |
| `provider` | 后端 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
