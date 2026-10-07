# provider/qiniu

这个目录只在进程启动时登记七牛 Modelink 的内容生成接口。它不转发 HTTP。转发是 `dataplane.ServeBypass`，匹配发生在 `provider.Match`。

`init` 调用三件事：

1. `RegisterSupplier`。显示名 `Qiniu`，slug `qiniu`，默认根 `https://api.qnaigc.com`。添加模型时的占位符是 `qiniu/bytedance/doubao-seedance-2-0-260128`。凭据字段是 `api_base`（默认同上）和必填的 `api_key`。
2. `RegisterType`。id 是 `qiniu_contents_generation`，种类 `bypass`。只挂在供应商 `qiniu` 上。模型字段名 `model`，任务 id 字段名 `id`，去掉的前缀只有 `qiniu`。
3. `RegisterModel` 登记三个目录行，对外 id 是 `qiniu/` 加上下面的官方 id，端点类型都是 `qiniu_contents_generation`：
   - `bytedance/doubao-seedance-2-0-260128`
   - `bytedance/doubao-seedance-2-0-fast-260128`
   - `bytedance/doubao-seedance-2-0-mini-260128`

## 和火山方舟不是同一套 URL

七牛公开路径和上游路径都是：

| 动作 | 方法 | 路径 |
| --- | --- | --- |
| create | POST | `/v3/contents/generations/tasks` |
| get | GET | `/v3/contents/generations/tasks/{id}` |

这里没有 `/api`，也没有 list 动作。火山方舟在 `provider/volcengine`，路径是 `/api/v3/contents/generations/tasks`，并且有 list。两边的任务 id 不能拿去对方的路径上查：`sameEndpoint` 对这种错配返回 false，`serveBypassFollow` 写 HTTP 404 `unknown task`。

价格不在这个文件里写成美元常量。目录行只登记 id 和文档地址 `https://docs.modelink.ai/api/video-doubao-seedance-20`。部署上的 `input_cost_per_token` / `output_cost_per_token` 会盖过目录价。

`internal/provider/all` 用空白导入把这个包拉进进程。不导入 `all` 的测试二进制里不会出现这些端点。

这个目录不实现 Kling，也不实现自定义的 Suno 或 Tripo。那些走部署上的 `litellm_params.endpoint`（种类 `bypass`），不必再写一个 Go 包。
