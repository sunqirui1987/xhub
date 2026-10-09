# 开发与 AI 编程指南

[文档首页](../README.zh-CN.md) · [架构](architecture.md) · [测试](testing.md)

专题参考：[全功能实现](implementation.md) · [模型与端点协议](model-endpoints.md) · [HTTP 接口](api-reference.md) · [配置与存储](configuration.md) · [路由](routing.md) · [护栏与 XGo 脚本](guardrails.md) · [权限](permissions.md) · [计价](pricing.md) · [运行边界](runtime.md) · [运行排查](operations.md) · [全链路回归](regression.md) · [浏览器回归](e2e-regression.md)。[internal 中文目录导航](../../internal/README.zh-CN.md)逐层指向每个包的实现与对外接口。

先确定修改影响的用户行为，再定位模块。文档用于解释边界，代码和有效测试用于核对当前实现；遇到冲突，应修正对应文档，避免用旧方案补出未实现的能力。

## 开始一次修改

1. 查看 `git status --short`，阅读修改目录适用的 `AGENTS.md`、`CLAUDE.md` 和包说明。保留已有工作，避免把无关文件纳入修改。
2. 用 `rg` 搜索入口、处理器和测试。接口从 `internal/gateway/routes.go`、`engine.go` 和各子模块的注册入口追到数据面；不要只看页面或路由目录。
3. 明确调用身份、资源归属、模型能力、失败行为和数据写入点。权限变化对照[权限规则](permissions.md)，路由和费用变化分别对照[路由](routing.md)、[计价](pricing.md)。
4. 实现最小的完整行为变更。补充能证明用户结果和失败边界的测试，避免只重复内部实现。
5. 按[测试指南](testing.md)运行受影响用例；涉及数据库时检查跳过项。改变控制台使用的后端接口后运行 `npm run gen:api`，不要手改生成文件。
6. 同步客户步骤和开发规则，说明实际执行的检查及仍未验证的部分。

## 按任务找代码

| 要改什么 | 先看哪里 | 相关参考 |
| --- | --- | --- |
| 登录、会话、密码 | `internal/auth`、`internal/iam/users.go`、`internal/gateway/identity` | [权限](permissions.md) |
| 组织、团队、项目、列表可见性 | `internal/authz`、`internal/iam/teams.go`、`internal/gateway/identity` | [权限](permissions.md) |
| 虚拟密钥、服务密钥 | `internal/gateway/keys`、`internal/iam/keys.go` | [权限](permissions.md) |
| 模型、供应商、端点能力 | `internal/gateway/models`、`internal/provider`、`internal/llm` | [协议边界](runtime.md) |
| 模板、选路、重试 | `internal/gateway/templateauth`、`internal/gateway/prefs`、`internal/router`、`internal/dataplane` | [路由](routing.md) |
| usage、价格、费用明细 | `internal/catalog`、`internal/gateway/spend.go`、`internal/iam/usage.go`、`internal/live` | [计价](pricing.md) |
| 页面、表单、国际化 | `frontend/src/app`、`frontend/src/components`、`frontend/src/i18n` | [frontend/CLAUDE.md](../../frontend/CLAUDE.md) |
| 配置、构建、启动 | `internal/config`、`configs`、`deploy/build.sh`、`Dockerfile` | [安装](../getting-started.zh-CN.md) |

## 必须保持的约束

- 控制台隐藏按钮不能替代服务器授权；每次请求重新检查身份和当前成员关系。读取父作用域或模板失败不能当作权限成功或默认策略。
- 列表在查询层限制范围；用户提供的筛选只能缩小范围。无团队范围和平台全局范围必须区分。
- 模型权限逐层收窄；路由模板不能扩大权限或绕过预算。个人密钥和服务密钥的所有权不能混用。
- 失败、流中断不能写成功缓存或成功粘性。流已输出后不能再拼接另一供应商的响应。
- usage 中明确的零和字段缺失要区分；缓存读 token 是输入 token 的子集。历史费用读取价格快照，不能用当前价格覆盖旧账。
- PostgreSQL 用量写入依赖事务和 `request_id` 去重；先提交数据库，再确认 Redis 队列。
- 对有副作用的创建操作，未知发送结果不能直接换部署重做。进程内幂等不能当作跨实例保证。
- 表单能保存字段不表示数据面会执行。新增供应商或路径登记不表示完整协议已经验证。

## 前端修改

遵循 [frontend/CLAUDE.md](../../frontend/CLAUDE.md)。不要把凭据写进 `localStorage`；不要手改 `src/lib/http/schema.d.ts`。修改相关英文和中文文案。路由模板选择器异步加载不得覆盖用户草稿，清空表示继承，零权重不得被默认值改成一。

Next.js 版本以 `frontend/package.json` 为准，修改框架相关代码前阅读当前安装包的 `node_modules/next/dist/docs/`。测试按受影响文件运行，避免无范围执行整个 Vitest 套件。

## 文档改动

客户指南只写用户需要完成的操作和实际限制。开发文档记录稳定规则、责任模块和测试入口。不要提交 AI 工作过程、完成清单、一次审查结论或临时测试报告；有长期价值的发现应归入对应规则。
