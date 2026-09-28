# gateway/identity

## How to use

Call the HTTP paths and the Go entry points in the sections below. Authenticate with the master key or an admin session unless a route is public.

## Purpose

`identity` is the HTTP API for users, teams, organizations, projects, and budgets. The dashboard screens for those objects call these routes. The package does not import the gateway process. The process implements `Gate` and mounts `Module`.

## HTTP paths you call

Authenticate with the master key or an admin session.

Users:

- `POST /user/new` creates a user. `GET /user/list` lists users.
- `GET /user/info` reads one user. `POST /user/update` edits the user. `POST /user/delete` deletes users.
- `GET /user/available_roles` lists the roles an admin may assign.

Teams:

- `POST /team/new` creates a team. `GET /team/list` and `GET /v2/team/list` list teams. The v2 route is paged.
- `GET /team/info?team_id=team_...` returns `team_info` and `team_memberships`. The dashboard detail page reads `team_info`.
- `POST /team/update` and `POST /team/delete` edit or remove a team.
- `POST /team/member_add`, `/team/member_update`, and `/team/member_delete` change membership.

Organizations, projects, and budgets follow the same new, list, info, update, and delete pattern under `/organization`, `/project`, and `/budget`.

Spend views:

- `GET /spend/logs` and `GET /global/spend` summarize spend for the admin screens.

## Example

```bash
curl -s "http://127.0.0.1:4000/team/info?team_id=team_19b51fd9ce95" \
  -H "Authorization: Bearer sk-local-master"
```

A 404 means that team id is not in the database this process is using. Check `search_path` if you pointed the process at the `e2e` schema.

## What this package does not do

It does not send inference traffic and it does not hash virtual keys. Key plaintext is `gateway/keys`.

中文使用说明见同目录的 readme_cn.md。
