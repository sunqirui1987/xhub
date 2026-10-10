# Bypass OpenAI 图片生成

## 原厂协议与认证

使用 XHub 公开模型名及虚拟密钥；部署必须启用对应 Bypass 协议。网关替换上游模型名并注入部署凭据，保留原厂扩展字段，同时执行权限、护栏和计费。客户端发送 Authorization: Bearer $XHUB_API_KEY。模型支持由部署决定，不表示所有原厂模型自动可用。

## 接口概述

图片生成

### 图片生成

## 接口行为
图片链接可能过期，按上游保留规则保存结果，计价可能按张或 Token，以费率为准。

## 接口与认证

~~~http
POST /bypass/openai/v1/images/generations
~~~

使用 XHub API 密钥：Authorization: Bearer <XHUB_API_KEY>。不要把供应商密钥当作网关密钥。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| model | string · required | 公开模型名，部署必须支持此协议。 |
| prompt | string · required | 图片描述。 |
| n / size / quality | integer / string | 输出数量、尺寸及质量，范围依上游。 |
| response_format | string | 支持时为 url 或 b64_json。 |

## 请求示例

~~~shell
curl "$XHUB_BASE_URL/bypass/openai/v1/images/generations" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","prompt":"A small lighthouse at sunrise.","n":1}'
~~~

## 成功响应

~~~json
{
  "created": 1677610602,
  "data": [
    {
      "url": "https://example.com/generated.png"
    }
  ]
}
~~~

| 字段 | 类型 |
| --- | --- |
| created | number |
| data | object[] |
| data[].url | string |

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。
