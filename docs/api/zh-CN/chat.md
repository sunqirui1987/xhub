# 对话补全

## 接口概述

POST /v1/chat/completions：发送消息。

### 请求字段

model 为公开模型名，messages 为 role 和 content 消息数组。使用 JSON 与 Bearer 认证。stream 默认非流式；可选参数需部署与上游支持。

### 响应与流式

非流式读取 choices 与 usage。stream=true 使用 SSE 直到结束。流开始后的错误不能改变 HTTP 状态，应查看事件与日志。

### 调用追踪

x-litellm-call-id 用于查日志，x-litellm-response-cost 为本地美元费用而非持久化提交回执，费用细节以日志快照为依据。

## 协议行为
普通响应读取 choices[].message，流式读取 choices[].delta。工具由应用执行并在下一轮发送结果。SSE 发出响应头后不能修改状态。POST /chat/completions 为别名。

## 接口与认证

~~~http
POST /v1/chat/completions
~~~

使用 XHub API 密钥：Authorization: Bearer <XHUB_API_KEY>。不要把供应商密钥当作网关密钥。

Base URL 使用调试台显示的网关根地址，不额外追加 /v1；实际请求使用上面的完整接口路径。

## 请求参数

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| model | string · required | 公开模型名，部署必须支持此协议。 |
| messages | object[] · required | role、content；工具结果含 tool_call_id，多模态内容需支持。 |
| stream | boolean | 默认 false；true 返回以 [DONE] 结束的 SSE。 |
| temperature / top_p | number | 采样，范围依模型。 |
| max_tokens / max_completion_tokens | integer | 输出上限不是消耗，字段依模型。 |
| tools / tool_choice | array / string or object | 函数定义及选择，调用方负责执行。 |
| response_format | object | 仅部署支持时可用结构化输出。 |

### 消息与工具嵌套字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| messages[].role | string | system、developer、user、assistant 或 tool，具体角色需模型支持。 |
| messages[].content | string 或 object[] | 文本或多模态内容；assistant 工具调用可附带 null 内容。 |
| messages[].content[].type | string | 兼容视觉模型使用 text 或 image_url；其他媒体类型取决于适配器。 |
| messages[].content[].text | string | 文本输入。 |
| messages[].content[].image_url.url | string | 上游支持的公网图片 URL 或 data URL。 |
| messages[].content[].image_url.detail | string | 上游支持时使用 auto、low、high，不保证映射到 Gemini 媒体分辨率。 |
| messages[].tool_call_id | string | 工具结果必须关联 assistant 返回的调用 ID。 |
| messages[].tool_calls[].id | string | assistant 工具调用标识。 |
| messages[].tool_calls[].function.name | string | 函数名称。 |
| messages[].tool_calls[].function.arguments | string | JSON 编码参数，应用执行工具前应校验。 |
| tools[].type | string | function。 |
| tools[].function.name | string | 应用定义的函数名。 |
| tools[].function.description | string | 可选函数说明。 |
| tools[].function.parameters | object | 参数 JSON Schema。 |
| tool_choice | string 或 object | 上游支持时可选 auto、none、required 或指定函数。 |
| response_format.type | string | 上游支持时可选 json_object 或 json_schema。 |
| response_format.json_schema | object | 上游支持的 schema、name 与 strict 配置。 |

### 流式响应与能力核对

stream 默认 false。stream=true 时消费 SSE data 事件，按索引累积 choices[].delta.content 和工具参数片段，到 [DONE] 结束。usage 可能缺失，依赖上游支持。响应头已发送后，错误可能出现在事件中，不能改写 HTTP 状态。

reasoning_effort、reasoning、thinking 的格式依模型而异。生图扩展、image_config、safety_settings、media_resolution 和 file_id 映射不属于 XHub 的统一保证。原厂 Bypass 保留相应上游协议；转换路径以实际适配器为准，不能假定同一参数在 OpenAI、Anthropic、Gemini 等供应商之间通用。

## 请求示例

~~~shell
curl "$XHUB_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","messages":[{"role":"user","content":"Hello, introduce yourself."}],"stream":false}'
~~~

## 成功响应

~~~json
{
  "id": "chatcmpl-example",
  "object": "chat.completion",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello!"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 5,
    "total_tokens": 15
  }
}
~~~

| 字段 | 类型 |
| --- | --- |
| id | string |
| object | string |
| choices | object[] |
| choices[].index | number |
| choices[].message | object |
| choices[].message.role | string |
| choices[].message.content | string |
| choices[].finish_reason | string |
| usage | object |
| usage.prompt_tokens | number |
| usage.completion_tokens | number |
| usage.total_tokens | number |

## 错误与排查

| HTTP | 说明 |
| --- | --- |
| 400 | 请求无效，请检查 JSON、参数和查询。 |
| 401 | XHub 凭据缺失或无效。 |
| 403 | 权限拒绝，请检查模型范围或策略。 |
| 429 | 超过限制，请检查响应详情与 Retry-After。 |

## 能力与计费边界

参数支持取决于部署模型及转换或 Bypass 路径；XHub 不保证所有上游扩展字段在所有模型中可用。以已实现的协议适配和上游响应为准。usage 的明细属于对应输入/输出总量，不能重复相加；费用以网关日志保存的计费快照为准。示例响应仅用于理解字段。
