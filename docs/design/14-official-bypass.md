# 14 添加供应商和模型

各目录的端点类型表和模型对应表：

- [provider](../../internal/provider/readme_cn.md)
- [all 导入](../../internal/provider/all/readme_cn.md)
- [OpenAI 适配](../../internal/provider/openai/readme_cn.md)
- [火山](../../internal/provider/volcengine/readme_cn.md)
- [七牛](../../internal/provider/qiniu/readme_cn.md)

添加一个模型有两条路。

接口形状已经在目录里：添加模型时选这个 Bypass 类型。火山内容生成和七牛内容生成是这样。

接口形状只在供应商文档里：添加模型时选「自定义 Bypass」，把文档上的格子填进去。Suno 和 Tripo 是这样，不写 Go。

## 1. 目录

一家供应商一个目录。这个目录登记端点类型、模型和单价。

```
internal/provider/
  type.go
  registry.go
  all/all.go                  每个供应商包一行空导入
  openai/chat.go              协议适配：chat、embedding、image、audio、rerank、video
  volcengine/seedance.go      Bypass：方舟内容生成
  qiniu/seedance.go           Bypass：七牛内容生成
```

希望下拉里以后都有这家接口时，新建 `internal/provider/<供应商>/<接口>.go`，再在 `all/all.go` 加一行空导入。gateway 和 dataplane 不用改。

`model_info.mode` 是这条部署的端点类型。空着等于 `chat`，仍走 `POST /v1/chat/completions`。

## 2. 端点类型的格子

添加模型的顺序是供应商、模型、定价、端点类型。端点类型分三组：协议适配、Bypass、自定义 Bypass。

选中 Bypass 或自定义之后，下面这些格子对应文档里的位置，都可以改。改过的一份存在 `litellm_params.endpoint`。

| 格子 | 文档里看哪 | 例子 |
| --- | --- | --- |
| API Base | Base URL | `https://api.tripo3d.ai` |
| 模型字段 | 正文里表示模型的字段。有的叫 `model`，Tripo 叫 `type` | `type` |
| 任务 id 字段 | 创建响应里 id 的路径 | `data.task_id`、`data.taskId`、`id` |
| 方法 | 这一节的 Method | `POST` 或 `GET` |
| 对外路径 | 调用方访问网关的路径，也是转给上游的路径 | `/v2/openapi/task` |
| 任务 id 查询参数 | id 不在路径里、在查询串里时的参数名。没有就空着 | Suno 的 `taskId` |

价目表行可以带 `endpoint_type`。选中 `volcengine/doubao-seedance-2-0-260128` 时，端点类型预填方舟内容生成。

供应商仍来自 `GET /public/providers/fields`。模型名和单价仍来自价目表。部署上另填的每百万 token 单价优先扣费。

## 3. 火山和七牛

这两家是内置 Bypass。

方舟内容生成，类型 `ark_contents_generation`，根地址 `https://ark.cn-beijing.volces.com`。

| 动作 | 方法 | 路径 | 模型字段 | 任务 id |
| --- | --- | --- | --- | --- |
| 创建 | POST | `/api/v3/contents/generations/tasks` | `model` | 响应 `id` |
| 查询 | GET | `/api/v3/contents/generations/tasks/{id}` | 无 | 路径 `{id}` |
| 列表 | GET | `/api/v3/contents/generations/tasks` | 无 | 无 |

模型 id `volcengine/doubao-seedance-2-0-260128` 发给方舟时去掉 `volcengine/`。价目表这一行是 $7.00 / 百万 token。方舟在线推理、480p 和 720p、输入不含视频的账单是 46 元 / 百万 token。1080p、4K、输入含视频在部署定价里填。fast 没有列表价。

七牛内容生成，类型 `qiniu_contents_generation`，根地址 `https://api.qnaigc.com`。创建是 `POST /v3/contents/generations/tasks`，查询是 `GET /v3/contents/generations/tasks/{id}`。`qiniu/bytedance/doubao-seedance-2-0-260128` 去掉 `qiniu/`，留下 `bytedance/`。fast 和 mini 同样。

创建响应里的任务 id 钉住这条部署 7 天。查询走这根钉。列表没有 id：这条密钥只有一个上游密钥就用那一个，并保留原来的查询串。有多个上游密钥时返回 400 和模型名。用量在查询响应第一次出现 `usage` 时扣一次。

## 4. 照 Tripo 文档填

文档：POST `https://api.tripo3d.ai/v2/openapi/task`，正文用 `type`，任务 id 在 `data.task_id`，查询是 GET `/v2/openapi/task/{task_id}`。

添加模型：

- 供应商选一个已有的，或先在目录里登记 Tripo。只想先打通时，供应商可以选带 API Key 和 API Base 的那一家，根地址再改成 Tripo 的。
- 模型填 `text_to_model`。调用方在 `type` 里写这个名字。
- 端点类型选自定义 Bypass。
- API Base：`https://api.tripo3d.ai`
- 模型字段：`type`
- 任务 id 字段：`data.task_id`
- 第一行方法 `POST`，对外路径 `/v2/openapi/task`
- 再加一行方法 `GET`，对外路径 `/v2/openapi/task/{task_id}`，任务 id 查询参数空着

调用：

```http
POST /v2/openapi/task
Authorization: Bearer <网关密钥>

{ "type": "text_to_model", "prompt": "a small cat" }
```

网关把密钥换成这条部署的 Tripo 密钥，`type` 换成部署上的模型 id，其余原样转发到 `https://api.tripo3d.ai/v2/openapi/task`。

## 5. 照 Suno 文档填

`https://docs.sunoapi.org` 的生成接口是 POST `https://api.sunoapi.org/api/v1/generate`，模型字段是 `model`，任务 id 在 `data.taskId`。查询不是路径参数，是 GET `/api/v1/generate/record-info?taskId=`。

自定义 Bypass：

- API Base：`https://api.sunoapi.org`
- 模型字段：`model`
- 任务 id 字段：`data.taskId`
- 创建：`POST /api/v1/generate`
- 查询：`GET /api/v1/generate/record-info`，任务 id 查询参数填 `taskId`

调用方查询时仍带 `?taskId=`。网关用这个参数找到创建时的部署，再把查询串原样转走。
