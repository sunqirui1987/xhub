# gateway/identity

账号、组织、团队、项目和审计列表的管理路由。由 `gateway/routes.go` 里的 `identity.Module` 挂上，早于嵌入的目录，所以这些路径赢过 `routes.json` 里的同名路径。

这个包不导入 `gateway`。它向 `Gate` 要 `RequireUser`、`Authorize` 和 `Identity()`。`*Server` 在 `wire.go` 里实现这个门。拒绝的判定经 `WriteIAMError` / `WriteAuthz` 变成 401、403 或 404。不存在的对象和调用方不拥有的对象都是 404。处理函数不说明是哪一种。

## 会改行的路由

| 方法和路径 | 处理函数 | 写什么 |
| --- | --- | --- |
| POST `/user/new` | `UserNew` | 平台账号。角色经 `storedRolePtr` 收成 `admin` 或 `user`。 |
| POST `/user/update`、POST `/user/delete` | `UserUpdate`、`UserDelete` | 资料、封禁或删除。改密码还走 POST `/user/set_password` 和 POST `/user/{user_id}/password`。 |
| POST `/organization/new` | `OrgNew` | 组织。只有平台管理员。 |
| POST `/team/new` | `TeamNew` | 团队。显示名是 `team_alias`，否则 `team_name`。正文没指定管理员时，调用方成为第一位 `team_admin`。 |
| POST `/team/member_add` | `TeamMemberAdd` | 成员。邮箱依次取 `user_email`、`user_id`、`email`。角色 `team_admin` 或 `admin` 收成 `iam.TeamAdmin`。其余包括平台角色都收成 `iam.TeamMember`。 |
| POST `/team/member_delete` | `TeamMemberRemove` | 按用户 id 或邮箱移除。塞进 `user_id` 的邮箱当成邮箱，因为账号 id 不是地址。最后一位 `team_admin` 不能被移除。 |
| POST `/project/new` | `ProjectNew` | 团队下的项目。名称是 `project_alias`，否则 `project_name`。 |

GET `/team/list` 返回数组，这条路径一直如此。平台管理员看见全部团队。其他人看见自己所在的团队，行上带着自己的角色。GET `/team/info` 给控制台详情页附上 `members_with_roles`。

GET `/organization/list` 和成员添加路由是有的。权限仍然来自团队成员关系。`authz` 里没有组织管理员角色，也没有项目管理员角色。

GET `/audit/logs` 按最新在前读 `audit_logs`。这些行由 `iam` 写入。这个包不决定哪次管理动作要审计。

## 这个包不做什么

它不签发虚拟密钥（`gateway/keys`），也不列出模型（`gateway/models`）。它不在推理调用上查密钥预算。

English notes are in `readme.md` in this directory.
