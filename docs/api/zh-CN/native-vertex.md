# Bypass Vertex 内容生成

## 原厂协议与认证

使用 XHub 公开模型名及虚拟密钥；部署必须启用对应 Bypass 协议。网关替换上游模型名并注入部署凭据，保留原厂扩展字段，同时执行权限、护栏和计费。客户端发送 Authorization: Bearer $XHUB_API_KEY。模型支持由部署决定，不表示所有原厂模型自动可用。 模型由 URL 指定。同一模型路径另有 :streamGenerateContent 与 :countTokens。Vertex 项目与区域由管理员配置在上游地址中。

## 接口概述

Gemini / Vertex 原生协议

### Gemini / Vertex 原生协议

## 接口行为
路径使用 URL 编码的公开模型名，还支持 :streamGenerateContent、:countTokens。Vertex Bypass 使用 /bypass/vertex/v1/models/{model} 及相同操作后缀。上游 OAuth 凭据保留在部署设置。

## 接口与认证

~~~http
POST /bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent
~~~

使用 XHub API 密钥：Authorization: Bearer <XHUB_API_KEY>。不要把供应商密钥当作网关密钥。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| contents | object[] · required | role 与文本或支持的媒体 parts。 |
| generationConfig / tools | object / array | 生成设置及工具定义。 |

## 请求示例

~~~shell
curl "$XHUB_BASE_URL/bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"role":"user","parts":[{"text":"Hello, introduce yourself."}]}]}'
~~~

## 成功响应

~~~json
{
  "candidates": [
    {
      "content": {
        "role": "model",
        "parts": [
          {
            "text": "Hello!"
          }
        ]
      }
    }
  ],
  "usageMetadata": {
    "promptTokenCount": 10,
    "candidatesTokenCount": 5
  }
}
~~~

| 字段 | 类型 |
| --- | --- |
| candidates | object[] |
| candidates[].content | object |
| candidates[].content.role | string |
| candidates[].content.parts | object[] |
| candidates[].content.parts[].text | string |
| usageMetadata | object |
| usageMetadata.promptTokenCount | number |
| usageMetadata.candidatesTokenCount | number |

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。
