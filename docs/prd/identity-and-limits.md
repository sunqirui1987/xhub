# 身份与限额

## 链

```text
Organization
  └── Team
        └── Project（enable_projects_ui 时）
              └── Virtual Key（VerificationToken）
Internal User *—* Team（membership，可自带 budget）
End User（请求 body `user` / `end_user` 或头 `x-litellm-end-user-id`）
```

JSON 字段名不可改：`organization_id`、`organization_alias`、`team_id`、`team_alias`、`project_id`、`project_alias`、`user_id`、`user_email`、`user_role`、Key 的 `token`（哈希）、`key_alias`、`key_name`。

## 向下收紧

模型白名单、`max_budget`、`tpm_limit`、`rpm_limit`、`object_permission` **不能比上级更宽**。

例：Team.`models` = `[gpt-4o-mini]`，Key.`models` 含 `gpt-4o` → Chat 该模型 **401/403**，即使 Key 自己写了允许。  
Team.`max_budget` 耗尽 → 其下所有 Key 数据面 **429**，`error.type` 为预算类，带 `Retry-After`。

`soft_budget` 只告警（callback / 邮件），不拦截。`max_budget` 拦截。

`object_permission`：`mcp_servers`、`vector_stores`、`agents`、`search_tools`、`skills`、`blocked_tools`。空列表语义与现网一致（未限制 vs 全禁），用合同测试钉死，不要猜。

删除 Team / Key：先写入 `Deleted*` 归档（保留 spend 历史），再删原行，同一事务。失败则两边都不改。
