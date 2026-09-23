# 角色

| 角色 | 是谁 | 成功 | 失败 |
|---|---|---|---|
| API 调用方 | 业务应用，持有虚拟 Key `sk-` | 只改 `base_url` + `api_key` 即可用官方 OpenAI/Anthropic SDK 调 Chat 等数据面 | 无 Key/过期/超预算返回对应 401/429 信封 |
| proxy_admin | 平台管理员 | 侧栏全部可见，能建 Key、模型、团队、策略 | 管理写失败有明确 4xx 与审计 |
| internal_user | 内部开发者 | 按可见页面列表使用 Playground、自己的 Key、用量 | 越权菜单隐藏且 API 403 |
| Chat 终端用户 | `/chat` 的使用者 | 选模型对话、看自己的日志与用量 | 不能进入管理员设置；无模型时空态 |

`proxy_admin_viewer` / `internal_user_viewer` 为只读变体。团队 admin 只管理本团队资源。
