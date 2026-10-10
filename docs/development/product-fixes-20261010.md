# 产品修复与分层验证对应（2026-10-10）

本记录接续[全项目审查](project-review-20261010.md)，保留基线失败证据，说明修复后的行为及实际执行结果。编译错误 `undefined: authz.ObjectInference` 已修正：继承模板的密钥预览使用现有 `ActionInfer` 与 `ObjectModel`，显式读取模板和编辑草稿仍使用管理授权。

本轮完成关键产品问题修复和对应验证；不将候选源码清单、语句覆盖率或全套运行等同于所有文件的逐行证明。全项目 1217 个源码候选、30 个 internal 包及各关键业务链的现有入口见[总矩阵](project-review-matrix-20261010.md)；本表给出已修复行为的闭环证据，未逐行为人工确认的候选仍标为待细审。前端定向复验、真实供应商和生产运维边界按实际证据记录。已退休 MCP 页面/服务的历史组件测试仅验证保留组件，不能当作当前后台 MCP 支持的证据；当前 useMCPServers 测试验证空结果与无网络调用。

## 行为 → 实现 → 单元 → 后台 → 浏览器 → 文档

| 行为与结果 | 实现 | 最近层测试及边界 | 真实后台 regression | 浏览器 E2E | 同步文档 |
| --- | --- | --- | --- | --- | --- |
| R01：创建、改名、删除部署后默认分配和客户模板无悬空引用；事务失败不发布内存目录 | [models/admin.go](../../internal/gateway/models/admin.go)、[store/model_weights.go](../../internal/store/model_weights.go)、[router/weight_cleanup.go](../../internal/router/weight_cleanup.go) | [weight_cleanup_test.go](../../internal/router/weight_cleanup_test.go)：正常、空目录、单部署、全零、改名、损坏输入和不修改原文档；[store 测试](../../internal/store/model_weights_test.go)使用真实 DB 验证回滚及并发模板最新行 | [model_weights_test.go](../../cmd/regression/model_weights_test.go)：保存/改名/删除/剩余部署推理和计费 | [weighted-routing.spec.ts](../../frontend/e2e/weighted-routing.spec.ts)：相对权重、模板绑定、部署减少、刷新和请求 | [routing.md](routing.md)、[router 中文说明](../../internal/router/readme_cn.md) |
| R02：管理员清空缓存后下一次推理重新外发并计费；未授权清空保留缓存 | [gateway/access.go](../../internal/gateway/access.go)、[cache/cache.go](../../internal/cache/cache.go) | [cache_test.go](../../internal/cache/cache_test.go)：多项、重复清理、重新写入；[access_cache_test.go](../../internal/gateway/access_cache_test.go)：HTTP 授权及副作用 | [consistency_test.go](../../cmd/regression/consistency_test.go) 的 TestCacheHitIsFreeAndLoggedAsOne：miss→免费 hit→拒绝清空→管理员清空→计费 miss | [product-fixes.spec.ts](../../frontend/e2e/product-fixes.spec.ts)：缓存页面点击清空，核对真实调用、日志和费用 | [implementation.md](implementation.md)、本矩阵 |
| R03：预览与推理按密钥→团队→组织选同一模板；会话仅在唯一团队时继承；预览不占 RPM/TPM | [gateway/limits.go](../../internal/gateway/limits.go)、[route_preview.go](../../internal/gateway/route_preview.go)、[route_settings.go](../../internal/gateway/route_settings.go) | gateway 身份解析及模板用例覆盖个人密钥、零/单/多团队、重置状态、缺失父级及存储错误；[selection_test.go](../../internal/gateway/templateauth/selection_test.go)验证可信归属和拒绝 | [route_preview_test.go](../../cmd/regression/route_preview_test.go)：真实会话/密钥预览、模板/模型权限、非法输入、限流与推理/计费一致 | [product-fixes.spec.ts](../../frontend/e2e/product-fixes.spec.ts)：页面打开模板 JSON 并保存；会话从唯一团队继承，密钥使用自身绑定，分别通过无显式参数的真实 preview API 与 inference 比对。页面预览本身提交显式草稿，不能把它写成隐式预览证据 | [routing.md](routing.md)、[templateauth 中文说明](../../internal/gateway/templateauth/readme_cn.md) |
| R04：预算/限流/白名单拒绝保留零费用错误日志，不增加成功、token 或金额；失败用例不污染后续场景 | [acceptance_limits_test.go](../../cmd/regression/acceptance_limits_test.go)、[harness_test.go](../../cmd/regression/harness_test.go) | 测试夹具绑定当前子测试上下文，恢复预算；单元细节由 IAM 限流和记账测试核对 | TestAcceptanceLimitsAndAccountingContract 的预算/RPM/TPM/白名单场景实际完成 | [acceptance-surfaces.spec.ts](../../frontend/e2e/acceptance-surfaces.spec.ts)、[error-logs.spec.ts](../../frontend/e2e/error-logs.spec.ts) | [implementation.md](implementation.md)、[testing.md](testing.md) |
| R05：路由巡检以真实 API 判定有数据/空态，独立无团队账户分别证明聊天日志和用量空态 | [coverage.spec.ts](../../frontend/e2e/coverage.spec.ts)，不修改产品报表 | 页面组件测试验证渲染；运行器数据隔离及报告契约用 Python 单元验证 | 用户隔离及分页 regression 验证可见数据范围 | coverage 的全路由巡检与 fresh isolated user 用例；账号删除及 schema 销毁 | 本矩阵与[基线问题](project-review-20261010.md) |
| 分页：空目录返回数组；极大页码不溢出；总数、稳定顺序和角色范围正确 | [gateway/codec.go](../../internal/gateway/codec.go)、[family/codec.go](../../internal/gateway/family/codec.go)、internal/pagination 与 IAM 目录 | gateway/family/identity pagination_test.go、[user_pagination_test.go](../../internal/iam/user_pagination_test.go)、[reports_test.go](../../internal/gateway/usage/reports_test.go)：第一页/末页/空值/非法值/大整数与角色 | [pagination_test.go](../../cmd/regression/pagination_test.go)：真实接口与独立普通用户身份，避免组织成员范围污染夹具 | [pagination.spec.ts](../../frontend/e2e/pagination.spec.ts)：会话日志合并分页、失败日志逐条分页、密钥/项目/项目密钥/审计 | [e2e-regression.md](e2e-regression.md)、本矩阵 |
| 冷却回归稳定性：先固定会话 A，再恢复 7:3 相对权重后失败 A，可靠验证转 B 和恢复 | [multi_supplier_test.go](../../cmd/regression/multi_supplier_test.go)，产品调度不变 | [router 测试](../../internal/router/readme_cn.md)覆盖调度细节；有限随机样本不保证严格 7:3 数量 | TestMultiSupplierCooldownIsolation：真实 Redis TTL、隔离、冷却及恢复，测试前后清理专用键 | [route-diagnostic.spec.ts](../../frontend/e2e/route-diagnostic.spec.ts) | [multi-supplier-regression.md](multi-supplier-regression.md)、[router/schedule.go](../../internal/router/schedule.go)注释 |
| 费用估算：折扣/利润优先级、token 价格、服务档位和 tokenizer 的正常/边界/失败输入 | [estimate/cost.go](../../internal/llm/estimate/cost.go)、[tokencount.go](../../internal/llm/estimate/tokencount.go)，本轮新增测试，不改变产品 | [cost_test.go](../../internal/llm/estimate/cost_test.go)、[tokencount_test.go](../../internal/llm/estimate/tokencount_test.go)：6 个顶层测试、24 个含子测试结果 | pricing、price_selection 及计费契约已有服务边界覆盖；新增纯函数用例不代替后台结算 | [price-model-selects.spec.ts](../../frontend/e2e/price-model-selects.spec.ts)间接覆盖目录价格链，不能证明真实供应商所有计费字段 | [estimate 双语说明](../../internal/llm/estimate/readme_cn.md) |
| 模板选择：空白表示继承；加载实际模板归属后授权；缺失/存储故障拒绝 | [templateauth/selection.go](../../internal/gateway/templateauth/selection.go)，本轮新增测试，不改变产品 | [selection_test.go](../../internal/gateway/templateauth/selection_test.go)：空输入纯单元；真实平台/组织/团队行与取消上下文服务边界 | route_template/binding 系列真实 HTTP 测试与 TestRoutePreviewMatchesInference | weighted-routing、route-diagnostic 和 product-fixes 的模板保存/绑定流程 | [templateauth 双语说明](../../internal/gateway/templateauth/readme_cn.md) |
| 报告契约：business 浏览器选择包含 19 个入口；mock 报告隔离，不覆盖真实报告 | [e2e-real-dataset.py](../../scripts/e2e-real-dataset.py)、[test_checklist.py](../../e2e/test_checklist.py)、[test_real_dataset.py](../../e2e/test_real_dataset.py) | scripts 31 个及 e2e 109 个测试通过；4 个集成前提跳过 | 驱动真实后台测试；mock 输出中的“真实请求”不等于实际供应商执行 | 真实浏览器报告单独保存 | [testing.md](testing.md)、[e2e-regression.md](e2e-regression.md) |

完整浏览器套件中的 3 个跳过为 mine-models 的未配置端点提示、model-endpoints 的配置恢复和 testdata-seed 的只构造资源场景；不将它们计为通过。

纯内部函数在最近层证明细节，浏览器在相关业务路径证明可观察结果。真实 DB 测试即使放在 internal 目录，也标为服务边界测试。测试只修改独立 schema、临时 Redis 和本轮对象；测试清理不针对用户配置。

## 已执行命令与结果

证据根目录为项目根 `.e2e/product-fixes-20261010/`，基线 `.e2e/review-20261010/` 保留。该目录被 Git 忽略，文档提交不会携带本机日志。计数按最终执行文件统计，不累加重跑次数；顶层与含子测试两种计数不混用。

| 实际命令（项目根运行，另注明者除外） | 通过 / 失败 / 跳过 | 证据 |
| --- | --- | --- |
| `go build -o .e2e/product-fixes-20261010/gateway ./cmd/gateway` | 构建成功 | build-final.log |
| `go vet ./internal/... ./cmd/regression` | 静态检查成功 | vet-complete.log |
| `XHUB_REGRESSION_STRICT=1 go test ./internal/... -count=1 -timeout=10m -json` | 顶层 539 / 0 / 2；含子测试 1473 / 0 / 2 | internal-rerun.jsonl |
| `XHUB_REGRESSION_REDIS_URL=redis://127.0.0.1:57418/0 XHUB_REGRESSION_STRICT=1 go test ./cmd/regression -count=1 -timeout=20m -json` | 顶层 184 / 0 / 8；含子测试 354 / 0 / 20 | regression-all-rerun.jsonl |
| `go test ./internal/llm/estimate -count=1 -json` | 顶层 6 / 0 / 0；含子测试 24 / 0 / 0 | estimate-unit-final.jsonl |
| `XHUB_REGRESSION_STRICT=1 go test ./internal/gateway/templateauth -count=1 -json` | 3 / 0 / 0；修正组织/团队夹具状态后的最终结果 | templateauth-pass3.jsonl |
| `python3 -m unittest discover -s scripts -p 'test_*.py'` | 31 / 0 / 0 | python-scripts.log |
| `python3 -m unittest discover -s e2e -p 'test_*.py'` | 109 / 0 / 4 | python-e2e-final.log |
| `go run ./cmd/gateway -h` | 入口编译及参数解析成功；未连接用户数据库 | gateway-run-help.log |
| frontend 中 `npm run gen:api` | 生成失败 1 次，未改写 schema | frontend/api-generation.log |
| frontend 中按明确文件运行 Vitest `--maxWorkers=2` | 88 文件归并 1128 / 1 / 0；仍有英文扫描失败 | frontend/summary-final.json；全部实际命令 frontend/commands-final.md |
| `E2E_BROWSER_REPORT_DIR=.e2e/product-fixes-20261010/full-final bash scripts/e2e.sh` | 131 / 0 / 3；0 flaky；完整 134 项，运行器中报告相对 frontend | browser-full-final/results.json、junit.xml（独立副本） |
| R03/R05 定向浏览器复验及产品/分页定向复验 | 分别 12 / 0 / 0 与 17 / 0 / 0；不与完整套件相加 | browser-r03-r05-final/results.json；browser-focused-final/results.json |
| website 中 `npm test` | 19 / 0 / 0 | website-unit-final.log |
| website 中 `npm run build` | 构建成功，101 篇文档及双语首页 | website-build-final.log |
| website 中 `npm run test:e2e` | 11 / 0 / 0；静态回归 104 页、13016 链接、101 索引文档，错误 0 | website-e2e-final.log；website-browser-results.json |

完整浏览器套件验证创建、编辑、预览、启用、拦截、脱敏、放行、计费与删除相关流程；缓存清空核对外发次数和日志/费用，模板验证会话团队继承与密钥自绑定，独立空账户验证真实 API 零数据及聊天用量/日志空态，分页验证实际列表边界。完整套件后的新增文案修改需按相关页面单独复验，不能把较早报告视作最终工作区全部通过。

全量 internal 运行后新增的 estimate/templateauth 用例已分别实跑，表中不把它们算入之前的全量数字。前端和浏览器计数按独立报告及文件最后一次执行归并，不以基线失败数字冒充最新结果。

分页、请求诊断及费用展示定向复验 10 文件归并 160 通过、0 失败；报告为 frontend/pagination-product-targeted-after.json 与 frontend/pagination-product-targeted-final.json。费用展示覆盖正常账单、零/空值/缓存边界和不可定价提示；请求诊断覆盖可配置错误、非 JSON、网络异常、429 及完整失败正文。不能将组件网络替身计为真实后台测试。

日志详情双语组件新增 5 个通过用例，覆盖聊天空状态、媒体成功/无响应、JSON 标签和失败正文；扫描器新增 4 个通过用例，确保混合中文、大小写英文及动态翻译键不会被宽泛排除。完整 translate.test.ts 最新 11 通过、1 失败，当前 85 处/30 文件候选经人工分为 23 处真实文案、40 处动态翻译/标识符、22 处技术或中文混合说明；人工分类不等于扫描通过。证据为 frontend/translate-final.json、english-hits-final.json 与 english-hits-final-classified.json。

## 文档一致性与修改范围

修正 development/testing、configuration、implementation、e2e-regression、routing、multi-supplier-regression，以及 internal/router、gateway/prefs、gateway/templateauth、llm、llm/estimate 双语说明。核对 llm、provider、config 及 gateway 当前入口，移除 20 个已退役函数引用（62 个 README、1064 个函数签名链接，最终失配 0）；当前文档审计核对 102 个 Markdown、2910 个相对链接，坏链 0；索引 749 个 Go 顶层测试、438 个唯一引用，活跃未解析引用 0；4 个命令前缀与5 个历史已删测试引用单独保留，不能计为当前缺失测试。证据 document-audit-final.json。删除指向已删除源码和测试的链接，纠正旧 model_routing/平台合并/固定权重比例描述；路由字段以 model_routes、allocations、retry_policy 的实际校验与执行为准。

产品与后台文件见上表链接；额外测试覆盖 gateway/identity、gateway/family、gateway/usage、iam 分页、缓存管理及新增估算/模板授权。前端文件包括 Teams、UsageTab、RegenerateKeyModal、模板/团队/组织视图、navbar、静态 schema 与估算、middleware；移除调用已删除 router_settings 模块的旧测试，现行路由设置由对应新模块测试及浏览器流程覆盖。[完整修改文件清单](product-fixes-files-20261010.md)包含基线以来同期分页及站点变更。

## 仍需明确的边界

- 内部 2 个及后台 8 个顶层真实供应商用例缺少对应外部凭据/可运行服务，未发起真实付费请求。已有本地服务覆盖配置、支持状态、错误、降级及数据面计费契约，不能据此证明远端字段漂移或真实媒体生成成功。
- 浏览器使用真实网关、PostgreSQL、临时 Redis 和本地协议供应商；“真实浏览器到数据面”与“实际外部 AI 服务”是不同证明范围。
- 多节点、重启期间请求、生产升级/备份恢复、长时间压力和完整故障注入未在本轮证明。
- `frontend` 中实际运行 `npm run gen:api` 失败：生成器依赖已退役的 Python `litellm.proxy.proxy_server`，尚未连接 Go 网关的接口来源。本轮未改写生成 schema，接口契约通过真实 HTTP regression 和消费者测试验证；Go 类型生成迁移仍未完成。证据为 frontend/api-generation.log。
- 基线前端 70 个失败文件中 69 个已定向通过或其旧入口已退休；270 个基线失败用例中 265 个所属文件闭环。translate 的 5 个基线失败不能全部计入清零：最新仍有 1 个生产英文扫描失败。本轮定向执行不等于整套前端全部通过，未人工逐行为核对的候选文件保留在审查清单。
