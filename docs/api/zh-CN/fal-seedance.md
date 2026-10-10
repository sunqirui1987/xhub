# Seedance

## 概述

此视频队列需要部署绑定对应 Fal transport。使用内容生成任务 transport 的 Seedance 部署不能直接通过 Fal 调用。

## 接口与认证

~~~http
POST /queue/bytedance/seedance-2.0/text-to-video
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
curl "$XHUB_BASE_URL/queue/bytedance/seedance-2.0/text-to-video" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"A lighthouse at sunrise.","duration":5}'
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
curl "$XHUB_BASE_URL/queue/bytedance/seedance-2.0/requests/$REQUEST_ID/status" -H "Authorization: Bearer $XHUB_API_KEY"
curl "$XHUB_BASE_URL/queue/bytedance/seedance-2.0/requests/$REQUEST_ID" -H "Authorization: Bearer $XHUB_API_KEY"
~~~

将创建响应 request_id 保存为 REQUEST_ID。查询状态直到完成，再取结果；优先将返回的 status_url、response_url 相对于网关根地址解析。查询使用创建时同一 XHub 密钥。取消未实现，成功终态及实测用量只结算一次。

## 已登记的创建路径

| Family / variant | Path |
| --- | --- |
| bytedance 2.0 | POST /queue/bytedance/seedance-2.0/text-to-video |
| bytedance 2.0 | POST /queue/bytedance/seedance-2.0/image-to-video |
| bytedance 2.0 | POST /queue/bytedance/seedance-2.0/reference-to-video |
| bytedance 2.0/fast | POST /queue/bytedance/seedance-2.0/fast/text-to-video |
| bytedance 2.0/fast | POST /queue/bytedance/seedance-2.0/fast/image-to-video |
| bytedance 2.0/fast | POST /queue/bytedance/seedance-2.0/fast/reference-to-video |
| bytedance 2.0/mini | POST /queue/bytedance/seedance-2.0/mini/text-to-video |
| bytedance 2.0/mini | POST /queue/bytedance/seedance-2.0/mini/image-to-video |
| bytedance 2.0/mini | POST /queue/bytedance/seedance-2.0/mini/reference-to-video |
| bytedance 2.5 | POST /queue/bytedance/seedance-2.5/text-to-video |
| bytedance 2.5 | POST /queue/bytedance/seedance-2.5/image-to-video |
| bytedance 2.5 | POST /queue/bytedance/seedance-2.5/reference-to-video |
| byteplus 2.0 | POST /queue/byteplus/seedance-2.0/text-to-video |
| byteplus 2.0 | POST /queue/byteplus/seedance-2.0/image-to-video |
| byteplus 2.0 | POST /queue/byteplus/seedance-2.0/reference-to-video |
| byteplus 2.0/fast | POST /queue/byteplus/seedance-2.0/fast/text-to-video |
| byteplus 2.0/fast | POST /queue/byteplus/seedance-2.0/fast/image-to-video |
| byteplus 2.0/fast | POST /queue/byteplus/seedance-2.0/fast/reference-to-video |
| byteplus 2.0/mini | POST /queue/byteplus/seedance-2.0/mini/text-to-video |
| byteplus 2.0/mini | POST /queue/byteplus/seedance-2.0/mini/image-to-video |
| byteplus 2.0/mini | POST /queue/byteplus/seedance-2.0/mini/reference-to-video |
| byteplus 2.5 | POST /queue/byteplus/seedance-2.5/text-to-video |
| byteplus 2.5 | POST /queue/byteplus/seedance-2.5/image-to-video |
| byteplus 2.5 | POST /queue/byteplus/seedance-2.5/reference-to-video |

图生视频和参考生成的媒体字段以部署端点契约为准。Fast/Mini 共用对应版本查询队列，不能在创建 URL 后直接拼接 /requests。byteplus 使用 /queue/byteplus/seedance-{version}/requests/{request_id} 查询。同路径绑定多个公开别名时，可添加 model 选择，网关转发前移除该字段。
