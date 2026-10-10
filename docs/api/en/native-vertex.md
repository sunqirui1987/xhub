# Bypass Vertex generateContent

## Native protocol and authentication

Use an XHub public model name and virtual key with the matching Bypass deployment. XHub maps the model and injects deployment credentials, preserves native fields and enforces access, guardrails and billing. Send Authorization: Bearer $XHUB_API_KEY. Model availability depends on configured deployments. The URL specifies the model. The model path also registers :streamGenerateContent and :countTokens. Administrators configure the Vertex project and region in the upstream address.

## Overview

Gemini / Vertex

### Gemini / Vertex

## Behavior
URL-encode the public model name in the path. Also supports :streamGenerateContent and :countTokens. Vertex Bypass uses /bypass/vertex/v1/models/{model} with the same operation suffixes. Provider OAuth credentials stay in deployment settings.

## Endpoint and authentication

~~~http
POST /bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent
~~~

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| contents | object[] · required | role and parts with text or supported media. |
| generationConfig / tools | object / array | Generation settings and tool declarations. |

## Request example

~~~shell
curl "$XHUB_BASE_URL/bypass/vertex/v1/models/YOUR_MODEL_NAME:generateContent" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"contents":[{"role":"user","parts":[{"text":"Hello, introduce yourself."}]}]}'
~~~

## Success response

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

| Field | Type |
| --- | --- |
| candidates | object[] |
| candidates[].content | object |
| candidates[].content.role | string |
| candidates[].content.parts | object[] |
| candidates[].content.parts[].text | string |
| usageMetadata | object |
| usageMetadata.promptTokenCount | number |
| usageMetadata.candidatesTokenCount | number |

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.
