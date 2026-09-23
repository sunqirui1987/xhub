# Guardrails

- Route: `/guardrails`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

内容策略 CRUD、测试、审批。

## 布局

- **顶栏 CTA**：无页级标题按钮。Guardrails Tab 内下拉 `Add New Guardrail`：Add Provider Guardrail / Create Custom Code Guardrail（仅 admin）。
- **筛选**：无列表搜索。详情用 `?guardrail=`。
- **表/卡片/Tab**：
  - Admin Tab：Guardrail Garden、Guardrails、Test Playground。所有人：Submitted Guardrails。
  - Guardrails 表列 Guardrail ID、Name、Provider、Mode、Default On、Created At、Updated At；行菜单 Delete。点 ID 进详情。
  - 详情 Tab：Overview、Settings（admin）。Garden 卡片选 provider 预置。Playground 测 allow/block/redact。
- **抽屉/模态**：AddGuardrailForm 向导（provider specific params）；CustomCodeModal；DeleteResourceModal。
- **URL query**：`?guardrail=<guardrail_id>` 打开详情（history push）。

## 交互

- Add Provider Guardrail → 向导 `POST /guardrails` → 新行。点 ID → `?guardrail=` Overview；Settings 保存 `PATCH /guardrails/{id}`。Delete → `DELETE`。Test Playground → `POST /guardrails/apply_guardrail` 展示 allow/block/redact。Submitted 供非 admin 提交、admin 审批。
- **view-only / 非 admin**：无 Garden / Guardrails / Playground Tab 与 Add；只见 Submitted Guardrails。详情无 Settings。
- **loading**：`Loading guardrails…`。
- **empty**：`No guardrails yet` / Add a guardrail to start filtering requests and responses.
- **forbidden**：非 admin 无写 CTA。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 | `GET /guardrails/list` | 表 |
| 创建 | `POST /guardrails` | 新行 |
| 更新 | `PATCH /guardrails/{guardrail_id}` | 保存 |
| 删除 | `DELETE /guardrails/{guardrail_id}` | 移除 |
| 测试 | `POST /apply_guardrail` | 展示 allow/block/redact |

## 字段

| 字段 | 含义 |
|---|---|
| `guardrail_name` | 名称 |
| `litellm_params.guardrail` | 提供商 |
| `mode` | pre_call/post_call |
| `default_on` | 默认开启 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
