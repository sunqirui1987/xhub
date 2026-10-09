# 模型部署、能力与价格管理

[English](readme.md) · [全功能实现说明](../../../docs/development/implementation.md)

## 职责与实现契约

list.go/available.go 提供按身份可见的模型；admin.go 和 validate.go 校验及持久化部署；builtin.go 从供应商目录导入；price.go、cost_reload.go 和 schedule.go 管理价格目录、来源、刷新及调度。
公开模型名与上游模型名分开；同名部署组成路由候选池。数据库部署覆盖 YAML。disabled 布尔状态同时影响发现和实际调用，enable/disable 不是删配置。命名凭据必须存在且供应商/协议匹配；连接字段可编辑。
价格来源只接受 catalog/manual；目录定价需要有效 base_model，手工定价需要明确费率。单价必须有限且非负，明确零价有效。端点能力和传输方式分别决定入口过滤与发送协议；拒绝已退休的 auto_router/adaptive_router 模型。管理返回隐藏上游秘密串，连接测试不能绕过模型权限。

## 源码职责与入口

### access.go

- [`func AllowsModel(s Host, ctx context.Context, p *auth.Principal, teamID, alias string) bool`](access.go) — AllowsModel is the model authorization decision shared by the model catalog and the inference path, so a listed model is always a usable model. The permitted set comes from iam alone, which resolves it down one chain: team = union of the team's active access groups project = team ∩ project narrowing (when the key names a project) key = (project or team) ∩ key narrowing (when the key narrows) teamI D narrows a session that is browsing one team's catalog; a key ignores it and answers for the single team it is bound to, never merging capabilities across teams. A scope with no grants at all is unrestricted: nothing has been assigned yet, and an empty assignment is not a denial.

### admin.go

- [`func Public(m config.ModelEntry) map[string]any`](admin.go) — Public is the model JSON shown to callers. Secrets inside the parameters are masked.
- [`func New(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — New creates a database model. A YAML model with the same name is then overridden by the database row.
- [`func Update(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Update changes a database model. Only fields present on the request are changed.
- [`func Delete(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Delete removes a database model. A model that exists only in YAML cannot be deleted.
- [`func Disable(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Disable prevents a stored deployment from being selected at runtime.
- [`func Enable(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — Enable makes a stored deployment eligible for selection.
- [`func CostMapSource(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — CostMapSource returns the price-map source, whether the built-in map is forced, the load time, and the model count.
- [`func GroupInfo(s Host, w http.ResponseWriter, r *http.Request)`](admin.go) — GroupInfo returns the deployments under one public model name.
- [`func LoadStored(s Host)`](admin.go) — LoadStored merges database models into this process's model table. Entries that exist only in the config file are not in this table, so after a restart they still come only from YAML and cannot be deleted from the page.

### available.go

- [`func Available(s Host, w http.ResponseWriter, r *http.Request)`](available.go) — Available serves model cards for the signed-in caller. It exposes only display metadata. A view-only session may read the models granted to it. Calling a model stays on AllowLLM.

### builtin.go

公开类型：`Builtin`, `CatalogModel`.

- [`func ParseModelIDs(body []byte) []string`](builtin.go) — ParseModelIDs reads an OpenAI models list. A body with no ids returns an empty slice.
- [`func ParseCatalog(body []byte) []CatalogModel`](builtin.go) — ParseCatalog maps an OpenAI-style models document to catalog cards.
- [`func ModelsURL(provider, base string) string`](builtin.go) — ModelsURL 拼出拉取上游模型目录的地址。供应商标识决定默认根路径。
- [`func SetBuiltinClient(c *http.Client)`](builtin.go) — SetBuiltinClient replaces the client used to fetch builtin model lists.
- [`func RefreshBuiltin(s Host, w http.ResponseWriter, r *http.Request)`](builtin.go) — RefreshBuiltin reloads the catalog. It does not add or delete models.
- [`func ListBuiltin(s Host, w http.ResponseWriter, r *http.Request)`](builtin.go) — ListBuiltin returns the specialized builtin catalog or model IDs discovered through a saved OpenAI-compatible credential. It does not write.
- [`func AddBuiltinModels(s Host, w http.ResponseWriter, r *http.Request)`](builtin.go) — AddBuiltinModels is the retired catalog-to-deployment shortcut. Catalog rows now enter /price/model and deployments are created through /model/new, so a caller cannot bypass the unified pricing editor.

### codec.go

内部实现和协议边界见 [codec.go](codec.go)。

### cost_reload.go

- [`func ReloadCostMap(s Host, w http.ResponseWriter, r *http.Request)`](cost_reload.go) — ReloadCostMap refetches the price catalog from the market feed now. On success it returns status and the model count. A failed fetch leaves the prices in use untouched and answers 502, so the console can say the reload did not happen instead of implying fresh prices arrived.
- [`func ScheduleCostMapReload(s Host, w http.ResponseWriter, r *http.Request)`](cost_reload.go) — ScheduleCostMapReload arms a reload every given number of hours. The hour count must be an integer from 1 to 168.
- [`func CancelCostMapReload(s Host, w http.ResponseWriter, r *http.Request)`](cost_reload.go) — CancelCostMapReload turns the timer off. The last-run time is kept.
- [`func CostMapReloadStatus(s Host, w http.ResponseWriter, r *http.Request)`](cost_reload.go) — CostMapReloadStatus reports whether the timer is on, its interval, and the last and next run times.

### host.go

公开类型：`Host`.

内部实现和协议边界见 [host.go](host.go)。

### list.go

- [`func List(s Host, w http.ResponseWriter, r *http.Request)`](list.go) — List serves GET /v1/models. Either an inference identity or a management identity is accepted. scope accepts only empty or expand; any other value returns 400. created uses the fixed LiteLLM default time, not the time the model was stored.
- [`func Info(s Host, w http.ResponseWriter, r *http.Request)`](list.go) — Info serves GET /v2/model/info, the deployment list behind the console's Models and Endpoints page and its auto-router lookups. The page reads this route and nothing else, so without a handler it fell through to the catalog's generic key-value store and answered an empty list — a page that looked like an empty deployment table rather than a missing endpoint. Each row is the deployment JSON the page renders, filtered to the models the caller may actually use so the table never offers a model the gateway would refuse. A platform administrator sees every deployment, matching the list route, where a missing grant means unrestricted rather than denied.

### mount.go

- [`func Module(h Host) httpx.Module`](mount.go) — Module is the model list and the management writes. The process implements Host.

### price.go

- [`func LoadPriceOverrides(s Host)`](price.go) — LoadPriceOverrides applies the stored catalog edits over the embedded baseline. It runs at startup, before the process serves traffic.
- [`func PriceList(s Host, w http.ResponseWriter, r *http.Request)`](price.go) — PriceList returns the live price catalog for the console: every model with the flag saying whether it came from the embedded file, and every supplier.
- [`func UpsertPriceModel(s Host, w http.ResponseWriter, r *http.Request)`](price.go) — UpsertPriceModel adds or edits one model in the price catalog. A row that the embedded file already has is overridden, not duplicated.
- [`func DeletePriceModel(s Host, w http.ResponseWriter, r *http.Request)`](price.go) — DeletePriceModel removes one model from the live catalog. The embedded file is not rewritten, so the deletion is remembered as a tombstone.
- [`func ResetPriceModel(s Host, w http.ResponseWriter, r *http.Request)`](price.go) — ResetPriceModel drops an override and restores the embedded row.
- [`func UpsertPriceProvider(s Host, w http.ResponseWriter, r *http.Request)`](price.go) — UpsertPriceProvider adds or edits one supplier in the add-model dropdown.
- [`func DeletePriceProvider(s Host, w http.ResponseWriter, r *http.Request)`](price.go) — DeletePriceProvider removes one supplier from the dropdown.

### schedule.go

- [`func StartScheduledReload(s Host)`](schedule.go) — StartScheduledReload runs the armed reload plan. It returns immediately when no plan is armed, and otherwise re-reads the plan each cycle so a console change takes effect without a restart.

### validate.go

内部实现和协议边界见 [validate.go](validate.go)。

## 对外 HTTP 边界

入口由下表所列注册文件安装。别名共享处理器；路径存在不替代权限和业务断言，授权说明见模块契约及接口参考。

| Method / path | 注册文件 |
| --- | --- |
| `GET /v1/models` | [mount.go](mount.go) |
| `GET /models` | [mount.go](mount.go) |
| `GET /model/available` | [mount.go](mount.go) |
| `GET /v2/model/info` | [mount.go](mount.go) |
| `GET /model/cost_map/source` | [mount.go](mount.go) |
| `POST /reload/model_cost_map` | [mount.go](mount.go) |
| `POST /schedule/model_cost_map_reload` | [mount.go](mount.go) |
| `DELETE /schedule/model_cost_map_reload` | [mount.go](mount.go) |
| `GET /schedule/model_cost_map_reload/status` | [mount.go](mount.go) |
| `GET /price/catalog` | [mount.go](mount.go) |
| `POST /price/model` | [mount.go](mount.go) |
| `DELETE /price/model` | [mount.go](mount.go) |
| `POST /price/model/reset` | [mount.go](mount.go) |
| `POST /price/provider` | [mount.go](mount.go) |
| `DELETE /price/provider` | [mount.go](mount.go) |
| `POST /model/new` | [mount.go](mount.go) |
| `POST /model/builtin/refresh` | [mount.go](mount.go) |
| `POST /model/builtin/models` | [mount.go](mount.go) |
| `POST /model/builtin/add` | [mount.go](mount.go) |
| `POST /model/update` | [mount.go](mount.go) |
| `PATCH /model/{model_id}/update` | [mount.go](mount.go) |
| `POST /model/delete` | [mount.go](mount.go) |
| `POST /model/disable` | [mount.go](mount.go) |
| `POST /model/enable` | [mount.go](mount.go) |
| `GET /model_group/info` | [mount.go](mount.go) |

## 依赖关系

[internal/auth](../../auth/readme_cn.md), [internal/catalog](../../catalog/readme_cn.md), [internal/config](../../config/readme_cn.md), [internal/httpx](../../httpx/readme_cn.md), [internal/iam](../../iam/readme_cn.md), [internal/llm](../../llm/readme_cn.md), [internal/logx](../../logx/readme_cn.md), [internal/store](../../store/readme_cn.md).

## 验证与维护入口

| 测试文件 | 场景入口 |
| --- | --- |
| [admin_test.go](admin_test.go) | `TestFailedModelUpdateDoesNotMutateStoredEntry`, `TestModelIDSearchFiltersBeforePagination`, `TestPublicDefaultsEnabledAndReportsDisabled`, `TestStripModelOwnership`, `TestManagementInfoIgnoresTeamFilterAndShowsDisabled` |
| [builtin_test.go](builtin_test.go) | `TestAddedModelMatchesHandAdded`, `TestPlaygroundGroupSkipsProviderShellsAndUsesChat`, `TestPlaygroundGroupsExcludeDisabledDeployments`, `TestParseModelIDs`, `TestBuiltinsEnabled`, `TestParseCatalogKeepsCategoryAndPrices`, `TestFillFromCostMapUsesPriceData`, `TestModelsURL`, `TestSlashedModelIDStaysTheModelName` |
| [list_test.go](list_test.go) | `TestProxyModelNamesSkipsProviderShells`, `TestProxyModelNamesSkipsLegacyProviderNamesWithoutRole`, `TestNonModelEntryKeepsOrdinaryModels`, `TestProxyModelNamesOmitsOnlyFullyDisabledNames` |
| [price_test.go](price_test.go) | `TestPricePartialUpdatePreservesQualifiedAndUntouchedRates`, `TestPriceCatalogServesTheEmbeddedBaseline`, `TestInvalidPriceDoesNotOverwriteStoredRates`, `TestPriceModelWriteIsStoredAndSurvivesAReload`, `TestPriceModelEditOverridesTheBaselineAndResetRestoresIt`, `TestPriceModelDeleteOfABaselineRowIsRemembered`, `TestPriceModelRejectsAMissingIdOrProvider`, `TestPriceWritesNeedManage`, `TestPriceProviderAddEditAndDeleteReachesTheDropdown`, `TestPriceProviderDeleteOfABaselineSupplierIsRemembered` |

```bash
go test ./internal/gateway/models -count=1
```

数据库验收设置 XHUB_REGRESSION_STRICT=1 并检查跳过项；Redis 和真实供应商需单独配置。接口、字段或行为改变后同步本说明及相关功能文档。
