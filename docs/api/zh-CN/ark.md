# 内容生成任务协议

## 接口概述

内容生成任务协议

### 内容生成任务协议

## 协议与适用范围
本文按内容生成任务协议组织，供应商只是部署实现。标准 /api/v3 路径支持集合 POST/GET 及 GET /api/v3/contents/generations/tasks/{id}；采用 /v3 变体的部署支持 POST /v3/contents/generations/tasks 与 GET /v3/contents/generations/tasks/{id}。路由上下文保留七天，不提供通用回调或取消，成功终态实测用量只结算一次。

## 接口与认证

~~~http
POST /api/v3/contents/generations/tasks
~~~

使用 XHub API 密钥：Authorization: Bearer <XHUB_API_KEY>。不要把供应商密钥当作网关密钥。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| model | string · 必填 | 绑定对应内容生成任务 transport 的公开模型别名。 |
| content | object[] · 必填 | 文本与媒体输入片段数组。 |
| content[].type | string | 按上游模型支持情况选择 text、image_url、video_url、audio_url。 |
| content[].text | string | text 片段的提示词，示例统一使用英文。 |
| content[].image_url.url | string | 图片引用地址，可访问性和格式遵循上游规则。 |
| content[].video_url.url / audio_url.url | string | 所选模型支持时传入视频或音频引用地址。 |
| content[].role | string | 媒体用途，如 first_frame、last_frame、reference_image、reference_video、reference_audio；不能假设所有模型都支持任意组合。 |
| duration | integer | 期望秒数，范围和智能时长语义取决于模型版本；不是实测计费用量。 |
| resolution / ratio | string | 部署支持的分辨率及画幅，不套用跨版本统一范围。 |
| generate_audio | boolean | 模型支持时请求同步音频。 |
| return_last_frame | boolean | 模型支持时请求尾帧结果。 |
| seed / watermark | integer / boolean | 可选上游生成控制，支持情况取决于所选模型。 |

## 请求示例

~~~shell
curl "$XHUB_BASE_URL/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","content":[{"type":"text","text":"A lighthouse at sunrise."}]}'
~~~

## 成功响应

~~~json
{
  "id": "task-example"
}
~~~

| 字段 | 类型 |
| --- | --- |
| id | string |

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。

## 任务查询与协议选择

将创建响应 id 保存为 TASK_ID，查询直到终态。内容生成任务协议使用 content 数组。部署绑定 Fal 时参见 [Seedance Fal](fal-seedance)，两种协议不能直接互换。

~~~http
GET /api/v3/contents/generations/tasks/{TASK_ID}
GET /v3/contents/generations/tasks/{TASK_ID}
~~~

## 查询响应字段

| 字段 | 说明 |
| --- | --- |
| id | 创建时返回的任务标识。 |
| status | 上游状态，常见 queued、running、succeeded、failed、expired；HTTP 200 不代表生成成功。 |
| content.video_url | 成功时的视频地址，按上游有效期及时保存。 |
| content.last_frame_url | 请求且支持时返回尾帧地址。 |
| error.code / error.message | 任务失败时查看错误码和原因。 |
| usage.completion_tokens / total_tokens | 结算所需实测用量，total 不是可以再次叠加的独立数量。 |
| created_at / updated_at | 上游返回的创建和更新时间。 |

~~~json
{
  "id": "task-example",
  "status": "succeeded",
  "content": {
    "video_url": "https://example.com/output.mp4"
  },
  "usage": {
    "completion_tokens": 1000,
    "total_tokens": 1000
  }
}
~~~

响应仅为示意。XHub 没有实现 Modelink 的临时素材创建、虚拟人审核、qvideo 任务转换、取消或回调结算。原厂扩展字段可能透传，支持情况以所部署上游契约为准。
