/** 已实现推理接口契约；示意响应的可选字段依供应商而异。 */
export const apiSpecs = [
  {
    id: "models",
    method: "GET",
    endpoint: "/models",
    anchor: "listModels",
    fields: [
      {
        name: "scope",
        type: "string",
      },
      {
        name: "team_id",
        type: "string",
      },
      {
        name: "return_wildcard_routes",
        type: "boolean",
      },
      {
        name: "only_model_access_groups",
        type: "boolean",
      },
    ],
    response:
      '{\n  "object": "list",\n  "data": [\n    {\n      "id": "YOUR_MODEL_NAME",\n      "object": "model",\n      "created": 1677610602,\n      "owned_by": "openai"\n    }\n  ]\n}',
    responseFields: [
      {
        name: "object",
        type: "string",
      },
      {
        name: "data",
        type: "object[]",
      },
      {
        name: "data[].id",
        type: "string",
      },
      {
        name: "data[].object",
        type: "string",
      },
      {
        name: "data[].created",
        type: "number",
      },
      {
        name: "data[].owned_by",
        type: "string",
      },
    ],
  },
  {
    id: "chat",
    method: "POST",
    endpoint: "/v1/chat/completions",
    anchor: "chat",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "messages",
        type: "object[] · required",
      },
      {
        name: "stream",
        type: "boolean",
      },
      {
        name: "temperature / top_p",
        type: "number",
      },
      {
        name: "max_tokens / max_completion_tokens",
        type: "integer",
      },
      {
        name: "tools / tool_choice",
        type: "array / string|object",
      },
      {
        name: "response_format",
        type: "object",
      },
    ],
    response:
      '{\n  "id": "chatcmpl-example",\n  "object": "chat.completion",\n  "choices": [\n    {\n      "index": 0,\n      "message": {\n        "role": "assistant",\n        "content": "Hello!"\n      },\n      "finish_reason": "stop"\n    }\n  ],\n  "usage": {\n    "prompt_tokens": 10,\n    "completion_tokens": 5,\n    "total_tokens": 15\n  }\n}',
    responseFields: [
      {
        name: "id",
        type: "string",
      },
      {
        name: "object",
        type: "string",
      },
      {
        name: "choices",
        type: "object[]",
      },
      {
        name: "choices[].index",
        type: "number",
      },
      {
        name: "choices[].message",
        type: "object",
      },
      {
        name: "choices[].message.role",
        type: "string",
      },
      {
        name: "choices[].message.content",
        type: "string",
      },
      {
        name: "choices[].finish_reason",
        type: "string",
      },
      {
        name: "usage",
        type: "object",
      },
      {
        name: "usage.prompt_tokens",
        type: "number",
      },
      {
        name: "usage.completion_tokens",
        type: "number",
      },
      {
        name: "usage.total_tokens",
        type: "number",
      },
    ],
  },
  {
    id: "responses",
    method: "POST",
    endpoint: "/v1/responses",
    anchor: "responses",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "input",
        type: "string|array · required",
      },
      {
        name: "instructions",
        type: "string",
      },
      {
        name: "max_output_tokens",
        type: "integer",
      },
      {
        name: "stream",
        type: "boolean",
      },
      {
        name: "tools / tool_choice",
        type: "array / string|object",
      },
      {
        name: "previous_response_id",
        type: "string",
      },
    ],
    response:
      '{\n  "id": "resp-example",\n  "object": "response",\n  "status": "completed",\n  "output": [\n    {\n      "type": "message",\n      "role": "assistant",\n      "content": [\n        {\n          "type": "output_text",\n          "text": "Hello!"\n        }\n      ]\n    }\n  ],\n  "usage": {\n    "input_tokens": 10,\n    "output_tokens": 5,\n    "total_tokens": 15\n  }\n}',
    responseFields: [
      {
        name: "id",
        type: "string",
      },
      {
        name: "object",
        type: "string",
      },
      {
        name: "status",
        type: "string",
      },
      {
        name: "output",
        type: "object[]",
      },
      {
        name: "output[].type",
        type: "string",
      },
      {
        name: "output[].role",
        type: "string",
      },
      {
        name: "output[].content",
        type: "object[]",
      },
      {
        name: "output[].content[].type",
        type: "string",
      },
      {
        name: "output[].content[].text",
        type: "string",
      },
      {
        name: "usage",
        type: "object",
      },
      {
        name: "usage.input_tokens",
        type: "number",
      },
      {
        name: "usage.output_tokens",
        type: "number",
      },
      {
        name: "usage.total_tokens",
        type: "number",
      },
    ],
  },
  {
    id: "messages",
    method: "POST",
    endpoint: "/v1/messages",
    anchor: "messages",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "max_tokens",
        type: "integer · required",
      },
      {
        name: "messages",
        type: "object[] · required",
      },
      {
        name: "system",
        type: "string|array",
      },
      {
        name: "tools / stream",
        type: "array / boolean",
      },
    ],
    response:
      '{\n  "id": "msg-example",\n  "type": "message",\n  "role": "assistant",\n  "content": [\n    {\n      "type": "text",\n      "text": "Hello!"\n    }\n  ],\n  "stop_reason": "end_turn",\n  "usage": {\n    "input_tokens": 10,\n    "output_tokens": 5\n  }\n}',
    responseFields: [
      {
        name: "id",
        type: "string",
      },
      {
        name: "type",
        type: "string",
      },
      {
        name: "role",
        type: "string",
      },
      {
        name: "content",
        type: "object[]",
      },
      {
        name: "content[].type",
        type: "string",
      },
      {
        name: "content[].text",
        type: "string",
      },
      {
        name: "stop_reason",
        type: "string",
      },
      {
        name: "usage",
        type: "object",
      },
      {
        name: "usage.input_tokens",
        type: "number",
      },
      {
        name: "usage.output_tokens",
        type: "number",
      },
    ],
  },
  {
    id: "embeddings",
    method: "POST",
    endpoint: "/v1/embeddings",
    anchor: "embeddings",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "input",
        type: "string|string[] · required",
      },
      {
        name: "encoding_format / dimensions",
        type: "string / integer",
      },
    ],
    response:
      '{\n  "object": "list",\n  "data": [\n    {\n      "object": "embedding",\n      "index": 0,\n      "embedding": [\n        0.1,\n        0.2\n      ]\n    }\n  ],\n  "usage": {\n    "prompt_tokens": 3,\n    "total_tokens": 3\n  }\n}',
    responseFields: [
      {
        name: "object",
        type: "string",
      },
      {
        name: "data",
        type: "object[]",
      },
      {
        name: "data[].object",
        type: "string",
      },
      {
        name: "data[].index",
        type: "number",
      },
      {
        name: "data[].embedding",
        type: "array",
      },
      {
        name: "usage",
        type: "object",
      },
      {
        name: "usage.prompt_tokens",
        type: "number",
      },
      {
        name: "usage.total_tokens",
        type: "number",
      },
    ],
  },
  {
    id: "completions",
    method: "POST",
    endpoint: "/v1/completions",
    anchor: "completions",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "prompt",
        type: "string|array · required",
      },
      {
        name: "max_tokens / temperature / stream",
        type: "integer / number / boolean",
      },
    ],
    response:
      '{\n  "object": "text_completion",\n  "choices": [\n    {\n      "index": 0,\n      "text": "Hello!",\n      "finish_reason": "stop"\n    }\n  ],\n  "usage": {\n    "prompt_tokens": 3,\n    "completion_tokens": 2,\n    "total_tokens": 5\n  }\n}',
    responseFields: [
      {
        name: "object",
        type: "string",
      },
      {
        name: "choices",
        type: "object[]",
      },
      {
        name: "choices[].index",
        type: "number",
      },
      {
        name: "choices[].text",
        type: "string",
      },
      {
        name: "choices[].finish_reason",
        type: "string",
      },
      {
        name: "usage",
        type: "object",
      },
      {
        name: "usage.prompt_tokens",
        type: "number",
      },
      {
        name: "usage.completion_tokens",
        type: "number",
      },
      {
        name: "usage.total_tokens",
        type: "number",
      },
    ],
  },
  {
    id: "images",
    method: "POST",
    endpoint: "/v1/images/generations",
    anchor: "images",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "prompt",
        type: "string · required",
      },
      {
        name: "n / size / quality",
        type: "integer / string",
      },
      {
        name: "response_format",
        type: "string",
      },
    ],
    response:
      '{\n  "created": 1677610602,\n  "data": [\n    {\n      "url": "https://example.com/generated.png"\n    }\n  ]\n}',
    responseFields: [
      {
        name: "created",
        type: "number",
      },
      {
        name: "data",
        type: "object[]",
      },
      {
        name: "data[].url",
        type: "string",
      },
    ],
  },
  {
    id: "image-edits",
    method: "POST",
    endpoint: "/v1/images/edits",
    anchor: "image-edits",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "image",
        type: "file|array · required",
      },
      {
        name: "prompt / mask",
        type: "string / file",
      },
    ],
    response: '{\n  "data": [\n    {\n      "b64_json": "BASE64_IMAGE_DATA"\n    }\n  ]\n}',
    responseFields: [
      {
        name: "data",
        type: "object[]",
      },
      {
        name: "data[].b64_json",
        type: "string",
      },
    ],
  },
  {
    id: "speech",
    method: "POST",
    endpoint: "/v1/audio/speech",
    anchor: "speech",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "input / voice",
        type: "string · required",
      },
      {
        name: "response_format / speed",
        type: "string / number",
      },
    ],
    response: '{\n  "note": "Binary audio response"\n}',
    responseFields: [
      {
        name: "note",
        type: "string",
      },
    ],
  },
  {
    id: "transcriptions",
    method: "POST",
    endpoint: "/v1/audio/transcriptions",
    anchor: "transcriptions",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "file",
        type: "file · required",
      },
      {
        name: "response_format / language",
        type: "string",
      },
    ],
    response: '{\n  "text": "Hello, welcome to XHub."\n}',
    responseFields: [
      {
        name: "text",
        type: "string",
      },
    ],
  },
  {
    id: "translations",
    method: "POST",
    endpoint: "/v1/audio/translations",
    anchor: "translations",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "file",
        type: "file · required",
      },
      {
        name: "response_format / language",
        type: "string",
      },
    ],
    response: '{\n  "text": "Hello, welcome to XHub."\n}',
    responseFields: [
      {
        name: "text",
        type: "string",
      },
    ],
  },
  {
    id: "moderations",
    method: "POST",
    endpoint: "/v1/moderations",
    anchor: "moderations",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "input",
        type: "string|array · required",
      },
    ],
    response:
      '{\n  "results": [\n    {\n      "flagged": false,\n      "categories": {},\n      "category_scores": {}\n    }\n  ]\n}',
    responseFields: [
      {
        name: "results",
        type: "object[]",
      },
      {
        name: "results[].flagged",
        type: "boolean",
      },
      {
        name: "results[].categories",
        type: "object",
      },
      {
        name: "results[].category_scores",
        type: "object",
      },
    ],
  },
  {
    id: "rerank",
    method: "POST",
    endpoint: "/v1/rerank",
    anchor: "rerank",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "query / documents",
        type: "string / array · required",
      },
      {
        name: "top_n",
        type: "integer",
      },
    ],
    response: '{\n  "results": [\n    {\n      "index": 0,\n      "relevance_score": 0.95\n    }\n  ]\n}',
    responseFields: [
      {
        name: "results",
        type: "object[]",
      },
      {
        name: "results[].index",
        type: "number",
      },
      {
        name: "results[].relevance_score",
        type: "number",
      },
    ],
  },
  {
    id: "gemini",
    method: "POST",
    endpoint: "/v1beta/models/YOUR_MODEL_NAME:generateContent",
    anchor: "gemini",
    fields: [
      {
        name: "contents",
        type: "object[] · required",
      },
      {
        name: "generationConfig / tools",
        type: "object / array",
      },
    ],
    response:
      '{\n  "candidates": [\n    {\n      "content": {\n        "role": "model",\n        "parts": [\n          {\n            "text": "Hello!"\n          }\n        ]\n      }\n    }\n  ],\n  "usageMetadata": {\n    "promptTokenCount": 10,\n    "candidatesTokenCount": 5\n  }\n}',
    responseFields: [
      {
        name: "candidates",
        type: "object[]",
      },
      {
        name: "candidates[].content",
        type: "object",
      },
      {
        name: "candidates[].content.role",
        type: "string",
      },
      {
        name: "candidates[].content.parts",
        type: "object[]",
      },
      {
        name: "candidates[].content.parts[].text",
        type: "string",
      },
      {
        name: "usageMetadata",
        type: "object",
      },
      {
        name: "usageMetadata.promptTokenCount",
        type: "number",
      },
      {
        name: "usageMetadata.candidatesTokenCount",
        type: "number",
      },
    ],
  },
  {
    id: "videos",
    method: "POST",
    endpoint: "/v1/videos",
    anchor: "videos",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "prompt",
        type: "string · required",
      },
      {
        name: "seconds / size / input_reference",
        type: "string / file",
      },
    ],
    response: '{\n  "id": "video-example",\n  "status": "queued"\n}',
    responseFields: [
      {
        name: "id",
        type: "string",
      },
      {
        name: "status",
        type: "string",
      },
    ],
  },
  {
    id: "ark",
    method: "POST",
    endpoint: "/api/v3/contents/generations/tasks",
    anchor: "ark",
    fields: [
      {
        name: "model",
        type: "string · required",
      },
      {
        name: "content",
        type: "object[] · required",
      },
      {
        name: "duration / resolution / ratio",
        type: "number / string",
      },
    ],
    response: '{\n  "id": "task-example"\n}',
    responseFields: [
      {
        name: "id",
        type: "string",
      },
    ],
  },
  {
    id: "fal",
    method: "POST",
    endpoint: "/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video",
    anchor: "fal",
    fields: [
      {
        name: "prompt",
        type: "string · required",
      },
      { name: "duration / aspect_ratio", type: "string" },
    ],
    response: '{\n  "request_id": "request-example"\n}',
    responseFields: [
      {
        name: "request_id",
        type: "string",
      },
    ],
  },
];
/** 独立队列文章复用 Fal 字段；路径来自注册表，供文档和主动运行，无网络副作用。 */
for (const [id, endpoint] of [
  ["fal-seedance", "/queue/bytedance/seedance-2.0/text-to-video"],
  ["fal-kling", "/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video"],
  ["fal-vidu", "/queue/fal-ai/vidu/q1/text-to-video"],
  ["fal-veo", "/queue/fal-ai/veo3.1"],
  ["fal-minimax", "/queue/minimax/h3-max/text-to-video"],
]) {
  const original = apiSpecs.find((item) => item.id === "fal")!;
  const queueSpec = { ...original, id, endpoint, anchor: id };
  apiSpecs.push(queueSpec);
}
/** 原厂文档复用协议字段；固定注册路径供目录与弹窗使用，无网络副作用。 */
for (const [id, source, endpoint] of [
  ["native-anthropic", "messages", "/bypass/anthropic/v1/messages"],
  ["native-vertex", "gemini", "/bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent"],
  ["native-gemini", "gemini", "/bypass/gemini/v1beta/models/YOUR_MODEL_NAME:generateContent"],
  ["native-images", "images", "/bypass/openai/v1/images/generations"],
  ["native-image-edits", "image-edits", "/bypass/openai/v1/images/edits"],
  ["native-responses", "responses", "/bypass/openai/v1/responses"],
  ["native-chat", "chat", "/bypass/openai/v1/chat/completions"],
]) {
  const original = apiSpecs.find((item) => item.id === source)!;
  const nativeSpec = { ...original, id, endpoint, anchor: id };
  apiSpecs.push(nativeSpec);
}
