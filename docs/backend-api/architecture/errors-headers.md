# 错误与响应头

## 请求头（数据面）

| Header | 必填 | 含义 |
|---|---|---|
| `Authorization` | 是* | `Bearer sk-...` |
| `x-litellm-api-key` | 否 | 若出现则优先于 Authorization |
| `Content-Type` | JSON 时是 | `application/json` 或 `multipart/form-data` |
| `Idempotency-Key` | 否 | 有则重放保护 |
| `x-litellm-tags` | 否 | 标签 |
| `x-litellm-end-user-id` | 否 | 终端用户 |

## 响应头（数据面成功）

| Header | 含义 |
|---|---|
| `x-litellm-call-id` | 调用 id |
| `x-litellm-model-id` | deployment id |
| `x-litellm-model-name` | 对外别名 |
| `x-litellm-model-api-base` | 上游 base（去 query） |
| `x-litellm-version` | 网关版本 |
| `x-litellm-response-cost` | USD |
| `x-litellm-response-cost-original` | 折扣前 |
| `x-litellm-response-cost-input` | 输入费 |
| `x-litellm-response-cost-output` | 输出费 |
| `x-litellm-response-cost-cache-read` | 缓存读 |
| `x-litellm-response-cost-cache-creation` | 缓存写 |
| `x-litellm-response-cost-reasoning` | 推理费 |
| `x-litellm-response-cost-tool-usage` | 工具费 |
| `x-litellm-key-tpm-limit` | Key TPM |
| `x-litellm-key-rpm-limit` | Key RPM |
| `x-litellm-key-max-budget` | Key 预算 |
| `x-litellm-key-spend` | Key 已花费 |
| `x-litellm-cache-key` | 缓存键 |
| `x-litellm-response-duration-ms` | 耗时 |
| `Retry-After` | 429 时 |

## 错误信封

OpenAI 兼容：

```json
{
  "error": {
    "message": "Budget has been exceeded",
    "type": "budget_exceeded",
    "param": null,
    "code": "429"
  }
}
```

Anthropic Messages：`{"type":"error","error":{"type","message"}}`。Gemini：Google 错误 JSON。
