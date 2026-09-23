# 文档完整性

PRD 必须能单独讲清网关怎么运转（两平面、鉴权、身份链、14 步、路由、SpendLogs、控制台）。契约与页面对分母的覆盖由 `docs/_tools/check_coverage.py` 门禁。

相对产品分母（数据面 + 管理 API + 控制台全页面 + 网关机制）：

| 维度 | 文档状态 |
|---|---|
| API | 每条 HTTP path 写在 `backend-api/contracts`，含请求头/请求体/响应头/响应体 |
| 前端 | 每个控制台路由含目的、布局、操作、字段 |
| 机制 | PROXY_HOOKS、鉴权决策、Router 策略默认、Spend flush、Provider URL |
| PRD | 五组导航 + Chat 壳 + 公开 Hub + 开通/登录；角色与成败路径 |

`mgmt.projects` / `mgmt.audit` / `mgmt.email` 等企业族保留在分母，L1 文件 Status 为 `independent-impl`，不得因 `http_routes` 扫描不到就删除。

覆盖检查：`python3 docs/_tools/check_coverage.py`（失败条件含 `操作 1`、`Status: missing`、handler 推诿、空 JSON、family http_paths 缺失、模糊 Provider 状态）。
