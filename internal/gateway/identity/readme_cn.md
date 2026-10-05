# gateway/identity

## 怎么使用

按下面的 HTTP 路径或 Go 入口调用。除了公开路由，请求都要带主密钥或管理员会话。

## 这个模块做什么

`identity` 是用户、团队、组织、项目和预算的 HTTP 接口。控制台这些页面调用这里的路由。这个包不引用网关进程。进程实现 `Gate` 并装上 `Module`。

## 你要调用的 HTTP 路径

用主密钥或管理员会话鉴权。

用户：

- `POST /user/new` 创建用户。`GET /user/list` 列出用户。
- `GET /user/info` 读取一个用户。`POST /user/update` 修改。`POST /user/delete` 删除。
- `GET /user/available_roles` 列出管理员可以分配的角色。

团队：

- `POST /team/new` 创建团队。`GET /team/list` 和 `GET /v2/team/list` 列出团队。v2 是分页的。
- `GET /team/info?team_id=team_...` 返回 `team_info` 和 `team_memberships`。控制台详情页读 `team_info.members_with_roles`。团队页能改什么，见 [团队管理](../../../docs/14-team-management.md)。
- `POST /team/update` 和 `POST /team/delete` 修改或删除团队。
- `POST /team/member_add`、`/team/member_update`、`/team/member_delete` 修改成员。

组织、项目和预算在 `/organization`、`/project`、`/budget` 下使用同样的 new、list、info、update、delete。

花费视图：

- `GET /spend/logs` 和 `GET /global/spend` 给管理页面做汇总。

## 例子

```bash
curl -s "http://127.0.0.1:4000/team/info?team_id=team_19b51fd9ce95" \
  -H "Authorization: Bearer sk-local-master"
```

404 表示这个进程使用的数据库里没有这个团队。如果进程连的是 `e2e` schema，先检查 `search_path`。

## 这个包不做什么

它不发送推理流量，也不哈希虚拟密钥。密钥明文在 `gateway/keys`。
