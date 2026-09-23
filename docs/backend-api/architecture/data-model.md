# 数据模型

必须存在的实体（表名可不同，JSON 字段名不可改）：

Organization、Team、Project、User、VerificationToken（Key）、EndUser、Budget、DeletedTeam、DeletedVerificationToken、ProxyModel、Credentials、Guardrail、Policy、PolicyAttachment、Prompt、Tag、AccessGroup、MCPServer、MCPToolset、MCPUserCredentials、SearchTool、VectorStore、Agent、Skill、Memory、UISettings、SSOConfig、CacheConfig、SpendLogs、Daily*Spend、ErrorLogs、AuditLog、HealthCheck、WorkflowRun/Event/Message、AutoRouter/AdaptiveRouter/ShadowEval、JWTKeyMapping、InvitationLink、TeamMembership、OrganizationMembership、ManagedFile、CronJob。

金额对外 USD float。删除 Team/Key 先归档再删。
