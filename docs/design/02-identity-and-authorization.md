# 02 身份与权限

> **目标设计，不是当前行为。** 本文描述的是计划要实现的架构。现在实际运行的行为见 [docs/current/](../current/)。两份冲突时，以 `current/` 和代码为准。

状态：已有身份实体、API key、会话及模型名单过滤；能力级授权、统一撤销和权限错误传播待实施。证据：internal/gateway/session.go 的 lookupSession、internal/gateway/models/access.go 的 effectiveScope / membershipScopes / entitiesForIDs，以及 internal/gateway/limits.go。

## 目标与边界

把“是谁”“在哪个作用域”“允许什么动作”分开决定。控制台会话可代表用户执行推理，但不能自动继承主密钥能力。API key 是用户委托的受限身份，不是绕过用户、团队或组织限制的入口。导航隐藏只是体验，服务端必须逐个接口授权。

本方案不重写 SSO 身份提供方；无论本地登录或 SSO，最终均映射到同一 Principal 和服务端会话状态。

## 身份契约

Principal 包含 kind、subject_id、session_id 或 key_id、可信 tenant scope、permission_version、认证时间和认证方式。用户、key、组织、团队、项目的 blocked、过期、撤销状态都参与决策。任意层显式拒绝优先于授权。

会话记录包含 token_digest 或 JWT jti、user_id、expires_at、revoked_at 和 permission_version。签名有效只证明令牌来源与完整性；还必须读取有效服务端记录并校验当前用户状态。注销、禁用账号、密码/成员权限变更后撤销或提升权限版本，旧会话必须失效。

缓存只能接受可证明版本仍有效的记录。普通 TTL 缓存意味着撤销有延迟，不能同时宣称立即撤销；需要即时撤销的入口必须读权威版本或可靠撤销索引。依赖失败返回 503 authorization_unavailable，不把读取错误当成“没有限制”。

## 能力矩阵

| 能力 | 普通用户 | 作用域管理员 | 系统管理员 |
| --- | --- | --- | --- |
| model.catalog.read | 看已发布目录 | 同左 | 同左 |
| model.available.read / inference.invoke | 显式授予，限定模型和作用域 | 显式授予 | 也需明确推理策略 |
| usage.read.self / logs.read.self | 自己的记录 | 自己的记录 | 自己的记录 |
| usage.read.team / logs.read.team | 默认无 | 管理的团队 | 授权范围内 |
| usage.read.all / logs.read.all | 无 | 无 | 显式授予 |
| model.catalog.write / provider.manage / credential.manage / pricing.manage | 无 | 仅委托的管理范围 | 显式授予 |

角色只是能力集合的模板，不能在 handler 中散落 admin 字符串判定。主密钥仅作为部署维护/应急身份，单独审计；不能让浏览器持有，也不能将普通会话解析为 master。

## 作用域与模型策略

请求必须绑定一个当前组织/团队/项目上下文。客户端可申请 scope_id，但服务端验证成员关系和实体层级后才接受；API key 的 scope 固定且不得扩大。多团队用户不应把 A 团队预算、B 团队模型及 C 团队凭据组合使用。

目标模型集合 = 已发布集合 ∩ 组织策略 ∩ 当前团队策略 ∩ 项目策略 ∩ 用户策略 ∩ key 委托策略，显式 deny 最后扣除。每层策略用 mode=inherit / allow_list / deny_all 表示：inherit 不增加限制，空 allow_list 明确表示无权，* 只展开到该作用域已发布别名。禁止用空数组同时表达“全部”和“一个都没有”。

当前 membershipScopes 会合并多个团队/组织的名单；迁移前需固定现有语义并生成差异预览，随后改为选定上下文求值。旧空名单如何映射必须由迁移规则明确，不能在上线时静默放大授权。

AuthorizationService.Decide(ctx, principal, action, resource, scope) 返回 allowed、reason_code、policy_version、resolved_scope，另行返回依赖错误。Available、模型选择器、推理入口、任务查询和报表使用同一个服务。关键实体不存在是拒绝；数据库不可用是服务错误，两者不能合并为 false 后返回空列表。

## 故障策略与保证

身份或权限读取不完整、撤销状态不确定、所属实体失效、层级关系冲突时，不启动上游。已启动任务的记账与恢复使用提交时快照继续执行；用户查询和下载仍按当前权限检查。

充分性：每次上游启动前都获得同一作用域的完整许可快照，所有限制取交集且拒绝优先，缺失任一权威读就拒绝，因此故障不能扩大模型集合。不可消除的边界：权限变更与外部提交不在同一事务；定义启动授权的线性化点为本地任务/attempt 获准提交的事务提交，之后撤销不追溯撤回已发生的外部工作。

最坏情况下数据库故障使新工作全部不可用。这是防越权的代价；缺少权威状态时无法区分“仍授权”与“已撤销”，任何持续放行都不能保证撤销生效。

## 迁移与验收

先增加会话记录、版本和能力表，再双读对比决策，不改变已有授权；差异确认后切换统一决策。无 DB 的开发模式必须显式启用、限制为本地测试，不作为生产授权默认。

验收：撤销会话/JWT、禁用用户/key、移出团队、交叉租户 ID、名单交集为空、权限库超时、旧版本缓存均不得启动上游。具有模型管理权而无 inference.invoke 的账号不能推理。日志/用量查询必须同时检查能力和对象归属。参见 [ADR-001](decisions/ADR-001-session-permissions.md)。
