# 产品需求（PRD）

PRD 是 **XHub 怎么运转** 的操作规格：两平面、鉴权顺序、身份链、一次数据面调用的阶段、路由与缓存、计量失败路径、控制台壳。读完本目录应能按字段与失败码实现网关，而不只是知道有哪些菜单。

逐 path 的 JSON 仍在 [后端契约](../backend-api/contracts/README.md)；逐页点击在 [前端](../frontend/README.md)。

| 文 | 讲什么 |
|---|---|
| [产品定义](product.md) | 分母：五组导航、Chat 壳、公开 Hub、数据面协议 |
| [怎么运转](runtime.md) | SDK 数据面 vs 控制台管理面；14 步生命周期；fallback 与首 chunk |
| [鉴权与虚拟 Key](auth-and-keys.md) | 头解析顺序、`VerificationToken`、`key_type`、明文 `sk-` 只一次 |
| [身份与限额](identity-and-limits.md) | Organization → Team → Project → User → Key；`max_budget` 向下收紧 |
| [路由与缓存](routing-and-cache.md) | `simple-shuffle` 等策略、重试/冷却、DualCache、`cache_hit` |
| [计量](spend.md) | 预扣再结算、`SpendLogs`、未知价格不得记 0 |
| [配置](config.md) | YAML `model_list` snapshot vs 库实体 |
| [控制台](console.md) | AI GATEWAY 等五组、默认 `/api-keys`、view-only、Chat 壳 vs Playground |
| [角色](actors.md) | 谁成功、谁失败 |
| [产品面验收](surfaces.md) | 表面级成败 |
| [术语](glossary.md) | 字段名 |
| [实现顺序](delivery.md) | 分期不缩小分母 |
