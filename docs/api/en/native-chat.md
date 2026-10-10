# Bypass OpenAI Chat Completions

## Native protocol and authentication

Use an XHub public model name and virtual key with the matching Bypass deployment. XHub maps the model and injects deployment credentials, preserves native fields and enforces access, guardrails and billing. Send Authorization: Bearer $XHUB_API_KEY. Model availability depends on configured deployments.

## Overview

POST /bypass/openai/v1/chat/completions: send messages.

### Request fields

model is the public name; messages contains role and content objects. Use JSON and Bearer authentication. stream defaults to non-streaming. Optional parameters require deployment and upstream support.

### Responses and streaming

Read choices and usage for non-streaming responses. stream=true uses SSE until completion. Errors after streaming starts cannot change HTTP status; inspect events and logs.

### Request tracing

Use x-litellm-call-id to find logs. x-litellm-response-cost is the locally calculated USD charge, not a persistence commit receipt. Inspect log snapshots for details.

## Protocol behavior
Read choices[].message for normal responses and choices[].delta for streaming. Tool calls require application execution and subsequent result messages. SSE errors after headers cannot change HTTP status. POST /chat/completions is an alias.

## Endpoint and authentication

~~~http
POST /bypass/openai/v1/chat/completions
~~~

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| model | string · required | Public model name; deployment must support this protocol. |
| messages | object[] · required | role and content; tool results include tool_call_id. Multimodal content requires support. |
| stream | boolean | False by default; true emits SSE ending with [DONE]. |
| temperature / top_p | number | Sampling; accepted ranges depend on model. |
| max_tokens / max_completion_tokens | integer | Output ceiling, not spent tokens; field depends on model. |
| tools / tool_choice | array / string or object | Function definitions and selection; caller executes tools. |
| response_format | object | Structured output only when deployment supports it. |

### Nested message and tool fields

| Field | Type | Meaning |
| --- | --- | --- |
| messages[].role | string | system, developer, user, assistant or tool, subject to model support. |
| messages[].content | string or object[] | Text or multimodal parts; null assistant content may accompany tool calls. |
| messages[].content[].type | string | For compatible vision models use text or image_url. Other media types depend on the protocol adapter. |
| messages[].content[].text | string | Text input. |
| messages[].content[].image_url.url | string | Public image URL or data URL when supported by the upstream model. |
| messages[].content[].image_url.detail | string | auto, low or high where supported; not a universal Gemini media-resolution mapping. |
| messages[].tool_call_id | string | Required for a tool result; matches the assistant tool call ID. |
| messages[].tool_calls[].id | string | Assistant tool call identifier. |
| messages[].tool_calls[].function.name | string | Function name. |
| messages[].tool_calls[].function.arguments | string | JSON-encoded arguments; validate before executing application tools. |
| tools[].type | string | function. |
| tools[].function.name | string | Function name supplied by the application. |
| tools[].function.description | string | Optional function description. |
| tools[].function.parameters | object | JSON Schema for arguments. |
| tool_choice | string or object | auto, none, required or a named function where supported. |
| response_format.type | string | json_object or json_schema where supported. |
| response_format.json_schema | object | Schema definition, name and strict setting where supported. |

### Streaming and capability checks

stream defaults to false. With stream=true, consume SSE data events, aggregate choices[].delta.content and tool call argument fragments by index, and stop at [DONE]. Usage may be absent unless the upstream supports it. An error after response headers were sent may appear in an event rather than as a new HTTP status.

Reasoning fields such as reasoning_effort, reasoning and thinking have model-specific formats. Image generation extensions, image_config, safety_settings, media_resolution and file_id mappings are not a universal XHub contract. Native Bypass preserves the corresponding upstream protocol; converted requests must be checked against the implemented adapter. Do not assume the same parameter works across OpenAI, Anthropic, Gemini or other providers.

## Request example

~~~shell
curl "$XHUB_BASE_URL/bypass/openai/v1/chat/completions" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","messages":[{"role":"user","content":"Hello, introduce yourself."}],"stream":false}'
~~~

## Success response

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

| Field | Type |
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

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.
