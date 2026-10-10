# Bypass OpenAI Responses

## Native protocol and authentication

Use an XHub public model name and virtual key with the matching Bypass deployment. XHub maps the model and injects deployment credentials, preserves native fields and enforces access, guardrails and billing. Send Authorization: Bearer $XHUB_API_KEY. Model availability depends on configured deployments.

## Overview

Responses

### Responses

## Behavior
POST /responses is an alias. Read output[].content. output_text is an SDK convenience, not a guaranteed raw field. Creation does not guarantee retrieval or cancellation routes.

## Endpoint and authentication

~~~http
POST /bypass/openai/v1/responses
~~~

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| model | string · required | Public model name; deployment must support this protocol. |
| input | string or array · required | Text or typed input items. |
| instructions | string | System instruction. |
| max_output_tokens | integer | Output ceiling. |
| stream | boolean | Typed SSE events when supported. |
| tools / tool_choice | array / string or object | Only tools supported by upstream. |
| previous_response_id | string | Requires upstream state support. |

## Request example

~~~shell
curl "$XHUB_BASE_URL/bypass/openai/v1/responses" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","input":"Hello, introduce yourself."}'
~~~

## Success response

~~~json
{
  "id": "resp-example",
  "object": "response",
  "status": "completed",
  "output": [
    {
      "type": "message",
      "role": "assistant",
      "content": [
        {
          "type": "output_text",
          "text": "Hello!"
        }
      ]
    }
  ],
  "usage": {
    "input_tokens": 10,
    "output_tokens": 5,
    "total_tokens": 15
  }
}
~~~

| Field | Type |
| --- | --- |
| id | string |
| object | string |
| status | string |
| output | object[] |
| output[].type | string |
| output[].role | string |
| output[].content | object[] |
| output[].content[].type | string |
| output[].content[].text | string |
| usage | object |
| usage.input_tokens | number |
| usage.output_tokens | number |
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
