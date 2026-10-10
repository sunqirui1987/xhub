# Vidu

## 概述

此视频队列需要部署绑定对应 Fal transport。使用内容生成任务 transport 的 Seedance 部署不能直接通过 Fal 调用。

## 接口与认证

~~~http
POST /queue/fal-ai/vidu/q1/text-to-video
~~~

使用 XHub API 密钥：Authorization: Bearer <XHUB_API_KEY>。不要把供应商密钥当作网关密钥。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| prompt | string · required | 提示词，已登记路径选择模型。 |
| duration / aspect_ratio | string | 合法值依操作。 |

## 请求示例

~~~shell
curl "$XHUB_BASE_URL/queue/fal-ai/vidu/q1/text-to-video" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"A lighthouse at sunrise.","duration":"5"}'
~~~

## 成功响应

~~~json
{
  "request_id": "request-example"
}
~~~

| 字段 | 类型 |
| --- | --- |
| request_id | string |

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。


## 创建、状态与结果

~~~shell
curl "$XHUB_BASE_URL/queue/fal-ai/vidu/requests/$REQUEST_ID/status" -H "Authorization: Bearer $XHUB_API_KEY"
curl "$XHUB_BASE_URL/queue/fal-ai/vidu/requests/$REQUEST_ID" -H "Authorization: Bearer $XHUB_API_KEY"
~~~

将创建响应 request_id 保存为 REQUEST_ID。查询状态直到完成，再取结果；优先将返回的 status_url、response_url 相对于网关根地址解析。查询使用创建时同一 XHub 密钥。取消未实现，成功终态及实测用量只结算一次。
