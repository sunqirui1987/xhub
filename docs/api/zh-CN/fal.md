# Fal 队列任务

## 接口概述

Fal 队列任务

### Fal 队列任务

## 接口行为
仅支持登记的 Fal 视频路径：Seedance、Kling、Vidu、Veo、MiniMax。创建返回 status_url 和 response_url，客户端相对于网关根地址解析，使用创建时同一 XHub 密钥查询。不能在创建路径后直接拼接 /requests/。

## 查询队列
| 系列 | 查询前缀 |
| --- | --- |
| Doubao Seedance 2.0 / 2.5 | /queue/bytedance/seedance-2.0/requests/ · /queue/bytedance/seedance-2.5/requests/ |
| Dreamina Seedance 2.0 / 2.5 | /queue/byteplus/seedance-2.0/requests/ · /queue/byteplus/seedance-2.5/requests/ |
| Kling | /queue/fal-ai/kling-video/requests/ |
| Vidu | /queue/fal-ai/vidu/requests/ |
| Veo 3.1 | /queue/fal-ai/veo3.1/requests/ |
| MiniMax H3 / H3 Max | /queue/minimax/h3/requests/ · /queue/minimax/h3-max/requests/ |

GET 前缀 + {request_id} 获取结果，GET 前缀 + {request_id}/status 获取状态。Fast/Mini 共用版本队列。同一路径有多个公开模型别名时，创建正文添加 model 选择，网关转发前删除该字段。

## 重试与结算
取消未实现，cancel_url 置空。创建网络错误或 5xx 不自动重放，避免重复付费任务；429 按路由策略处理。Webhook 透传不触发结算，需查询成功终态及实测用量。具体创建模式、字段及计价变体以部署端点详情为准，不提供任意 Fal 通配代理。

## 接口与认证

~~~http
POST /queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video
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
curl "$XHUB_BASE_URL/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video" \
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


## 按模型系列接入

- [Seedance](fal-seedance)
- [Kling](fal-kling)
- [Vidu](fal-vidu)
- [Veo 3.1](fal-veo)
- [MiniMax H3 Max](fal-minimax)
