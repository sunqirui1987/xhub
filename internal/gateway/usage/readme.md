# gateway/usage

Spend and activity routes for the console. Mounted as `usage.Module`. Every list is narrowed by `UsageScope` or `LogsScope` before filters run. A scope that matches nothing is a deny-all condition, not an empty list the client could mistake for "no traffic". Master-key callers get that deny-all scope.

## Logs

GET `/spend/logs/v2` and GET `/spend/logs/ui` are `LogsV2`. GET `/spend/logs/ui/{request_id}` is one row. The scope is the caller's own personal logs plus the service-key logs of teams they administer. A platform administrator may read another person's personal log, and that read is audited. Filters (`start_date`, `end_date`, model, key, team) only narrow inside the scope.

The row joins `usage_events` to `request_logs` when prompt storage was on. Headers, body, and response are the stored documents with `Authorization`, `Cookie`, and api-key headers already replaced by `***` at write time (`redactHeaders` in `spend.go`). Session grouping uses the session id stamped by `AnnotateCall`. GET `/spend/logs/session/ui` lists by that id.

Cost on the console is dollars. Token totals are shown in units of 万 with two decimal places in the UI. This package returns the raw integers and the dollar float. It does not format 万.

## Activity

GET `/user/daily/activity`, `/team/daily/activity`, and `/organization/daily/activity` fold `usage_events` in the caller's timezone. The query parameter `timezone` is the browser's `Date.getTimezoneOffset()` (positive west of Greenwich). `uiTimezone` negates it, so a value of 0 means UTC. Dates are `YYYY-MM-DD` after that shift. Events with cost 0 are dropped from the activity fold.

GET `/global/activity`, `/global/spend/teams`, `/global/spend/keys`, `/global/spend/models` are the admin roll-ups. They still apply the scope. A non-admin does not see every team by calling `/global/spend/teams`.

POST `/spend/calculate` prices a token count with `catalog.Cost`. An unknown model is not returned as zero.

## What this package does not do

It does not write the event. `gateway/spend.go` `recordSpend` and `dataplane.Flush` do that, through `iam.RecordUsage`. Creating an official task does not insert a billed event. The first follow-up that carries usage does, once.

中文说明见同目录 `readme_cn.md`。
