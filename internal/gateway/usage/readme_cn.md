# gateway/usage

控制台的花费和活动路由。以 `usage.Module` 挂上。每次列表都先套 `UsageScope` 或 `LogsScope`，再套筛选。什么都匹配不到的范围是恒假条件，不是一份会被客户端当成「没有流量」的空列表。主密钥调用方得到这个恒假范围。

## 日志

GET `/spend/logs/v2` 和 GET `/spend/logs/ui` 是 `LogsV2`。GET `/spend/logs/ui/{request_id}` 是一行。范围是调用方自己的个人日志，加上所管理团队的服务密钥日志。平台管理员可以读别人的个人日志，这次阅读会进审计。筛选（`start_date`、`end_date`、模型、密钥、团队）只能在范围里面收窄。

打开了提示词存储时，行把 `usage_events` 和 `request_logs` 接在一起。头、正文和响应是落库时的文档，`Authorization`、`Cookie` 和 api-key 头在写入时已被 `spend.go` 的 `redactHeaders` 换成 `***`。会话分组用 `AnnotateCall` 盖上的会话 id。GET `/spend/logs/session/ui` 按这个 id 列。

控制台上的费用是美元。token 合计在界面上按「万」、两位小数显示。这个包返回原始整数和美元浮点。它不格式化成「万」。

## 活动

GET `/user/daily/activity`、`/team/daily/activity`、`/organization/daily/activity` 按调用方时区折叠 `usage_events`。查询参数 `timezone` 是浏览器的 `Date.getTimezoneOffset()`（格林威治以西为正）。`uiTimezone` 取反，所以 0 表示 UTC。日期是偏移之后的 `YYYY-MM-DD`。费用为 0 的事件不进入活动折叠。

GET `/global/activity`、`/global/spend/teams`、`/global/spend/keys`、`/global/spend/models` 是管理员汇总。它们仍然套范围。非管理员调用 `/global/spend/teams` 看不见所有团队。

POST `/spend/calculate` 用 `catalog.Cost` 给 token 数计价。不认识的模型不会被返回成 0。

## 这个包不做什么

它不写事件。写事件的是 `gateway/spend.go` 的 `recordSpend` 和 `dataplane.Flush`，经 `iam.RecordUsage`。官方任务的创建不插入已计费事件。第一次带 usage 的后续查询才记，而且只记一次。

English notes are in `readme.md` in this directory.
