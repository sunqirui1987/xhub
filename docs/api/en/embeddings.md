# Embeddings

## Overview

Embeddings

### Embeddings

## Behavior
Example vectors are shortened. Match data[].index to each input and validate dimensions before indexing.

## Endpoint and authentication

~~~http
POST /v1/embeddings
~~~

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| model | string · required | Public model name; deployment must support this protocol. |
| input | string|string[] · required | Text or batch of texts. |
| encoding_format / dimensions | string / integer | float or base64; vector length only if supported. |

## Request example

~~~shell
curl "$XHUB_BASE_URL/v1/embeddings" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","input":["Hello world"]}'
~~~

## Success response

~~~json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [
        0.1,
        0.2
      ]
    }
  ],
  "usage": {
    "prompt_tokens": 3,
    "total_tokens": 3
  }
}
~~~

| Field | Type |
| --- | --- |
| object | string |
| data | object[] |
| data[].object | string |
| data[].index | number |
| data[].embedding | array |
| usage | object |
| usage.prompt_tokens | number |
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
