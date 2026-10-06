# gateway/usage

## 怎么使用

按下面的 HTTP 路径或 Go 入口调用。除了公开路由，请求都要带主密钥或管理员会话。

## 这个模块做什么

`usage` 给花费和活动页面提供数据。数字来自 PostgreSQL 里已经记下的花费行。打开页面不会重新计价。没有自动路由基准样本时，基准接口返回空列表，不编造分数。

## HTTP 路径

- `GET /spend/logs/v2` 列出请求日志。`cache_hit` 是布尔值。单条日志是 `GET /spend/logs/ui/{request_id}`。
- `GET /global/spend/keys`、`/global/spend/models`、`/global/spend/provider`、`/global/spend/teams`、`/global/spend/tags` 给图表做分组。
- `GET /user/daily/activity` 和 `/user/daily/activity/aggregated` 是「你的用量」和「用户用量」。`GET /team/daily/activity/aggregated` 是「团队用量」。`GET /organization/daily/activity` 是「组织用量」。范围都是调用者自己的行，加上他管辖的团队。
- `GET /global/activity` 和 `/global/activity/cache_hits` 给活动面板。
- `POST /spend/calculate` 按你提交的 token 数估算成本。它不写日志行。
- `GET /auto_router/benchmarks` 返回已保存的基准行，没有则是空列表。

## 例子

```bash
curl -s "http://127.0.0.1:4000/spend/logs/v2?start_date=2026-09-01&end_date=2026-09-30" \
  -H "Authorization: Bearer sk-local-master"
```

管理员能看到每个用户的花费。非管理员只能看自己的用户 id。

## 这个包不做什么

它不给正在进行的聊天计价。那发生在数据面记录响应的时候。
