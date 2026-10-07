# provider

端点类型的目录。一个供应商一个子目录，在 `init` 里调用这里的登记函数。网关通过 `internal/provider/all` 的空白导入把子目录拉进来。这个包不监听端口，也不发出上游请求。

## 登记

- `RegisterSupplier` 记下显示名、slug、默认 API 根，以及添加模型时要填的凭据字段。slug 为空则忽略。
- `RegisterType` 记下一种端点。`KindAdapted` 走 `dataplane.Serve`。`KindBypass` 走 `dataplane.ServeBypass`。id 为空或没有动作则忽略。
- `RegisterModel` 记下添加模型表单里的一行。选中这个模型 id 时，表单带上 `EndpointType`。

自定义 bypass 不必再写 Go。部署的 `litellm_params.endpoint` 是一个对象：种类 `bypass`，加上 `model_field`、`task_id`、`strip_prefix`、`api_base` 和 `actions`（每项有 `name`、`method`、`public_path`、`upstream_path`、`task_query`）。`overrideType` 在匹配之后把这些字段盖到命中的类型上。公开路径仍以类型登记的为准，上游路径和模型字段以部署为准。Suno（`docs.sunoapi.org`）和 Tripo 就是这样填的，不在子目录里。

## 匹配

`Match(method, path)` 只匹配 bypass 动作。适配类型的 `/v1/chat/completions` 等仍由网关目录和 Gin 处理，不从这里返回。

`OfficialID` 只去掉一层供应商标前缀。`qiniu/bytedance/doubao-…` 发给上游时是 `bytedance/doubao-…`。`volcengine/doubao-seedance-2-0-260128` 发给上游时是 `doubao-seedance-2-0-260128`。

`SelectedTypes` 读 `model_info.endpoint_types`。这个数组非空时盖过 `model_info.mode`。都空则是 `chat`。`Includes` 判断一次请求的端点是否在部署选中的类型里。`BoundTypes` 把选中的 id 收成已登记的动作；选了 `custom` 时把部署上的自定义 bypass 算进去。

`ModeOf` 是选中类型的第一个，给还在用单个 `mode` 字段的旧调用方。

## 子目录

| 目录 | 登记了什么 |
| --- | --- |
| `openai` | 七种适配类型：chat、completion、embedding、image、audio、rerank、videos |
| `qiniu` | bypass `qiniu_contents_generation`，路径 `/v3/contents/generations/tasks`，没有 `/api` |
| `volcengine` | bypass `ark_contents_generation`，路径 `/api/v3/contents/generations/tasks` |
| `all` | 只有空白导入，没有自己的类型 |

七牛和方舟的 URL、模型 id 和有没有 list 动作都不相同。任务钉不能跨这两条路径用。
