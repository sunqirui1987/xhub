# gateway/models

## 怎么使用

按下面的 HTTP 路径或 Go 入口调用。除了公开路由，请求都要带主密钥或管理员会话。

## 这个模块做什么

`models` 列出调用方可以用的模型，并保存控制台的修改。PostgreSQL 里同名的模型覆盖 YAML。`openai/*` 这类通配符会在调用方真正看到的列表里展开。

## HTTP 路径

- `GET /v1/models` 和 `GET /models` 列出这把 bearer token 能看见的模型。
- `POST /model/new` 添加部署。`POST /model/update` 修改。`POST /model/delete` 删掉数据库行。
- `POST /model/block` 和 `POST /model/unblock` 隐藏或恢复一个模型。
- `GET /model/info` 返回公开的部署记录。
- `POST /reload/model_cost_map` 立刻重载价格。控制台认 `status`、`models_count` 和 `scheduled`。
- `POST /schedule/model_cost_map_reload` 和 `DELETE /schedule/model_cost_map_reload` 设定或取消定时重载。`GET /schedule/model_cost_map_reload/status` 报告定时器。

## 例子

```bash
curl -s http://127.0.0.1:4000/v1/models \
  -H "Authorization: Bearer sk-local-master"
```

`models` 只包含 `gpt-4o-mini` 的虚拟密钥只能看见这个名字，看不见整份目录。

## Go 调用方

进程装上 `models.Module`。`models.List` 是公开模型列表背后的处理函数。不要用 `catalog.CostMap` 决定一把密钥能调用什么。密钥、团队和用户上的允许列表在这个包里生效。
