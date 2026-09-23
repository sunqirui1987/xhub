# Skills

- Route: `/skills`
- Nav: AI GATEWAY
- Status: `specified`

## 目的

网关 Skills 与 Claude Code 插件。

## 布局

- **顶栏 CTA**：标题 Skills。说明发布后出现在 Skill Hub，经 `/claude-code/marketplace.json` 提供。`+ Add Skill`（非 admin disabled）。
- **筛选**：无独立筛选；表客户端按 Created At 排序。
- **表/卡片/Tab**：单表（非 Skills/Plugins 双 Tab）。列 Skill Name、Version、Description、Category、Public、Created At；admin 行菜单 Delete。点行进 `SkillDetail`（返回、发布/启用）。
- **抽屉/模态**：Add Plugin 表单（git/archive 源、路径、digest）。Delete Skill 确认。
- **URL query**：无。

## 交互

- 点 + Add Skill → 模态；提交 `POST /claude-code/plugins` → 新行。点行 → 详情；发布/enable → `POST /claude-code/plugins/{plugin_name}/enable`，Public/enabled 变 true。Delete → `DELETE /claude-code/plugins/{name}`。
- **view-only / 非 admin**：Add Skill disabled；行无 Delete。仍可读表与详情。
- **loading**：`Loading skills…`。
- **empty**：`No skills found` / Add one to get started.
- **forbidden**：非 admin 写操作按钮不可用；列表请求失败静默空表。
- 校验失败可改后重试；超时锁定原 payload。

## 操作 → 契约

| 用户动作 | Method Path | 成功后 UI |
|---|---|---|
| 列出 skills | `GET /v1/skills` | 表 |
| 创建 | `POST /v1/skills` | 新行 |
| 插件列表 | `GET /claude-code/plugins` | 插件表 |
| 启用插件 | `POST /claude-code/plugins/{plugin_name}/enable` | enabled=true |

## 字段

| 字段 | 含义 |
|---|---|
| `skill_id` | id |
| `name` | 名称 |
| `source` | 来源 |
| `enabled` | 是否启用 |
| `version` | 版本 |
| `category` | 分类 |

## 验收

桌面与窄屏走通上表每一行动作；不得 mock 表格。
