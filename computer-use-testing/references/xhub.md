# xhub 测试约定

从仓库根目录读取 AGENTS.md、docs/development/e2e-regression.md 和目标功能用例。执行前核对当前版本与环境，不默认启动整套验收。

- 单元：选择修改模块对应的 Go/Python/Vitest 用例。前端入口为 npm --prefix frontend run test:unit -- <筛选参数>。
- 后台：bash scripts/regression.sh -v '<实际存在的测试名模式>'，使用真实网关、隔离 PostgreSQL schema 和本地供应商。
- 浏览器：bash scripts/e2e.sh <实际存在的用例.spec.ts>，验证真实控制台、网关及隔离数据，先检查构建依赖和 PostgreSQL。
- 完整离线：make e2e-offline，仅在任务需要完整范围时使用。

make testdata 会清空指定 PostgreSQL 的 xhub/public 和 Redis DB 1，不能用于共享开发环境的临时建数。make e2e、make e2e-all 和 --live regression 调用真实供应商并产生费用，仅在当前任务明确授权范围内执行。

浏览器入口有锁、专属端口和隔离 schema。不得删除其他运行的锁或并发抢占 .e2e/current。手动 Computer Use 接入已声明的测试环境，使用专属产物目录；只使用明确提供的测试账号，不输出凭据。

按功能验证登录空值/错误/成功；资源创建→列表→编辑→刷新回读；取消不保存；权限拒绝；护栏试跑与保存后的实际拦截/脱敏/放行；密钥启停后的调用；删除后 UI 与后台均消失。

真实网关/数据库配本地供应商只证明离线协议与业务效果。真实模型输出、外部目录、DNS/TLS、媒体生成、付费结算的可用性另外记录。以资源 ID 或 call_id 关联页面、接口、日志与计量。

产物位于 .e2e/computer-use/<run-id>/。现有浏览器报告可能在 .e2e/current/ 或 .e2e/runs/，及时保存本轮证据避免覆盖。脚本负责自己的服务与 schema；手动资源另行记录和清理。
