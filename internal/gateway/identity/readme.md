# gateway/identity

Management routes for accounts, organizations, teams, projects, and the audit list. Mounted by `identity.Module` from `gateway/routes.go` before the embedded catalog, so these paths win over a same path in `routes.json`.

The package does not import `gateway`. It asks a `Gate` for `RequireUser`, `Authorize`, and `Identity()`. `*Server` implements that gate in `wire.go`. A refused decision becomes 401, 403, or 404 through `WriteIAMError` / `WriteAuthz`. A missing object and an object the caller does not own are both 404. The handler does not say which one it was.

## Routes that change rows

| Method and path | Handler | What it writes |
| --- | --- | --- |
| POST `/user/new` | `UserNew` | Platform account. Role is `admin` or `user` after `storedRolePtr`. |
| POST `/user/update`, POST `/user/delete` | `UserUpdate`, `UserDelete` | Profile, block flag, or removal. Password changes also use POST `/user/set_password` and POST `/user/{user_id}/password`. |
| POST `/organization/new` | `OrgNew` | Organization. Only a platform administrator. |
| POST `/team/new` | `TeamNew` | Team. Display name is `team_alias`, else `team_name`. The caller becomes the first `team_admin` when the body names nobody. |
| POST `/team/member_add` | `TeamMemberAdd` | Membership. Email is `user_email`, else `user_id`, else `email`. Role `team_admin` or `admin` becomes `iam.TeamAdmin`. Anything else, including a platform role, becomes `iam.TeamMember`. |
| POST `/team/member_delete` | `TeamMemberRemove` | Removes by user id or email. An email stuffed into `user_id` is treated as an email, because account ids are not addresses. The last `team_admin` cannot be removed. |
| POST `/project/new` | `ProjectNew` | Project under a team. Name is `project_alias`, else `project_name`. |

GET `/team/list` returns an array, which is what that path has always returned. A platform administrator sees every team. Anyone else sees the teams they belong to, with their own role on the row. GET `/team/info` attaches `members_with_roles` for the console detail page.

GET `/organization/list` and the member-add routes exist. Permission still comes from team membership. There is no organization-admin role and no project-admin role in `authz`.

GET `/audit/logs` reads `audit_logs` newest first. `iam` writes those rows. This package does not decide which admin action is audited.

## What this package does not do

It does not mint virtual keys (`gateway/keys`) and it does not list models (`gateway/models`). It does not check the key budget on an inference call.

中文说明见同目录 `readme_cn.md`。
