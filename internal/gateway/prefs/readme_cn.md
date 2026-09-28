# gateway/prefs

## 怎么使用

按下面的 HTTP 路径或 Go 入口调用。除了公开路由，请求都要带主密钥或管理员会话。

## 这个模块做什么

`prefs` 给路由设置页和通用设置页提供数据。你读到的值是 YAML 基线再盖上数据库行。数据库里出现的键胜出，即使值是列表或 null。数据库里没有的键保持 YAML 的值。

## HTTP 路径

- `GET /router/settings` 返回控制台要画的合并后路由文档。`GET /router/fields` 给字段编辑器返回同一份文档。
- `POST /config/update` 把一个字段写进数据库覆盖。列表和标量是替换。嵌套对象会合并。
- `GET /config/list` 返回通用设置视图。`master_key`、`database_url` 和 `redis_url` 不会被放进这个视图。
- 日志页用的回调列表挂在这些路由旁边。

## 例子

```bash
curl -s http://127.0.0.1:4000/config/update \
  -H "Authorization: Bearer sk-local-master" \
  -H "Content-Type: application/json" \
  -d '{"router_settings":{"routing_strategy":"least-busy"}}'
```

然后再 GET 一次路由设置。`routing_strategy` 会变成 `least-busy`，你没有提交的字段还在。

## Go 调用方

`prefs.MergedRouter` 和 `prefs.MergedGeneral` 返回 HTTP 处理函数写出的同一份文档。`Overlay` 把数据库里的键盖到 YAML 上。`MergePatch` 把局部更新折进当前文档。传入一个能读配置和存储的宿主。进程实现了这个宿主。
