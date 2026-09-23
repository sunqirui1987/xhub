# 产品面：主路径与失败路径

每个表面都必须有成功与失败。前端页面规格见 [frontend/pages](../frontend/pages/README.md)。

| 表面 | 角色 | 成功 | 失败 |
|---|---|---|---|
| 虚拟密钥 `/api-keys` | proxy_admin / internal_user | 创建后一次展示 `sk-`，随后 list 无明文 | 校验失败可改；超时不重复提交 |
| 调试台 `/playground` | 有写权限者 | SSE 打字、取消、工具、图片 | 无权限模型不出现；流失败展示 error.message |
| 模型与端点 | proxy_admin | 保存部署后 Chat 立即可用 | 测试连接失败不写假健康 |
| Agents / Workflows / Memory / MCP / Skills | 按权限 | CRUD 落库 | 测连接失败可见 |
| Guardrails / Policies / Tools | 按权限 | block 不打上游 | 超时按 fail policy |
| 用量 / 日志 / 监控 | admin + 内部用户（自己的数据） | 数字与 SpendLogs 一致 | 空区间空态 |
| 团队 / 用户 / 组织 / 预算 / 访问组 / 项目 | admin | 创建后 list 可读；白名单对 Chat 生效 | 跨租户 403 |
| Chat 壳 `/chat` | Chat 终端用户 | 对话走 `/v1/chat/completions` | 不能调管理写接口 |
| 公开 Hub `/model_hub` | 匿名或登录 | 浏览模型 | 无写按钮 |
| 登录 / 开通 / 连接 | 所有人 | session 建立 | 错密 401；过期邀请 400 |
| MCP OAuth 回调 | 浏览器 | 换 token 回 MCP 页 | state 不匹配拒绝 |
| 数据面 SDK | API 调用方 | 见后端各契约验收 | 未实现 Provider 明确错误 |

控制台不得用 mock 数据冒充已实现。
