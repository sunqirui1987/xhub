# catalog/publicdata

## 这个目录做什么

这里存放 `catalog` 嵌进网关二进制的 JSON。它们是数据，不是 Go 代码。改价格或控制台表单字段，就是改这里的 JSON 然后重新编译。运行中的进程不会监视磁盘上的这些文件。

## 文件

- `model_cost_map.json` 是内置价格表，`catalog.CostMap` 读它。表里没有的模型不能当成 0 美元。
- `autorouter_presets.json` 是控制台展示的自动路由预设。
- `provider_create_fields.json` 描述操作员添加供应商部署时，控制台要画的字段。
- `agent_create_fields.json` 描述创建智能体表单的字段。智能体产品面没有挂出来，但这份文档仍跟其它公开数据放在一起，字段定义不另找地方。

## 怎么改这些数据

改 JSON，保持合法，然后重新编译 `./cmd/gateway`。调用方不要用 `os.ReadFile` 打开这些文件。他们调用 `catalog.CostMap`、`catalog.PublicBody`，或者 `gateway/models` 里的重载函数。

不要在 JSON 里写注释。名字要保持稳定：控制台和价格估算按模型名、字段名精确查找。
