# provider/volcengine

这个目录只在进程启动时登记火山方舟的内容生成接口。它不转发 HTTP。转发是 `dataplane.ServeBypass`。

`init` 调用：

1. `RegisterSupplier`。显示名 `VolcEngine`，slug `volcengine`，默认根 `https://ark.cn-beijing.volces.com`。这个供应商登记没有凭据字段列表；密钥仍写在部署的 `api_key` 上。
2. `RegisterType`。id 是 `ark_contents_generation`，种类 `bypass`，只挂在 `volcengine`。模型字段 `model`，任务 id 字段 `id`，去掉的前缀只有 `volcengine`。
3. `RegisterModel` 两条：
   - `volcengine/doubao-seedance-2-0-260128`，官方 id `doubao-seedance-2-0-260128`。标了价。方舟这一档标价是人民币 46 元 / 百万 token；价格表用美元，所以这一行存的是 BytePlus 480p/720p、无视频输入的 7 美元 / 百万 token，也就是每个 token `7/1_000_000`，输入和输出相同。文档 `https://www.volcengine.com/docs/82379/1544106`。
   - `volcengine/doubao-seedance-2-0-fast-260128`，官方 id `doubao-seedance-2-0-fast-260128`。这一行没有在代码里写单价。文档 `https://www.volcengine.com/docs/82379/1520757`。

## 和七牛不是同一套 URL

方舟的公开路径和上游路径：

| 动作 | 方法 | 路径 |
| --- | --- | --- |
| create | POST | `/api/v3/contents/generations/tasks` |
| get | GET | `/api/v3/contents/generations/tasks/{id}` |
| list | GET | `/api/v3/contents/generations/tasks` |

七牛在 `provider/qiniu`，路径是 `/v3/contents/generations/tasks`，没有 `/api`，也没有 list。`OfficialID` 只剥一层前缀，所以发给方舟的模型名是 `doubao-seedance-2-0-260128` 这种，不会带 `volcengine/`。钉住的方舟任务 id 如果被拿到七牛路径上，`sameEndpoint` 返回 false，`serveBypassFollow` 写 HTTP 404 `unknown task`。

创建请求不扣费。第一次带 usage 的后续查询才记一次，键 `official_billed:v1:`，钉的有效期 7 天（`gateway/affinity.go` 的 `officialPinTTL`）。部署上的 `input_cost_per_token` / `output_cost_per_token` 盖过上面的目录价。

`internal/provider/all` 用空白导入把这个包拉进进程。
