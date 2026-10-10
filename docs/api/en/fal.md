# Fal queue tasks

## Overview

Fal queue tasks

### Fal queue tasks

## Behavior
Only registered Fal video paths are supported: Seedance, Kling, Vidu, Veo and MiniMax. Resolve returned status_url and response_url against the gateway root and query with the same XHub key used to create the task. Do not append /requests/ to the creation path.

## Query queues
| Family | Query prefix |
| --- | --- |
| Doubao Seedance 2.0 / 2.5 | /queue/bytedance/seedance-2.0/requests/ · /queue/bytedance/seedance-2.5/requests/ |
| Dreamina Seedance 2.0 / 2.5 | /queue/byteplus/seedance-2.0/requests/ · /queue/byteplus/seedance-2.5/requests/ |
| Kling | /queue/fal-ai/kling-video/requests/ |
| Vidu | /queue/fal-ai/vidu/requests/ |
| Veo 3.1 | /queue/fal-ai/veo3.1/requests/ |
| MiniMax H3 / H3 Max | /queue/minimax/h3/requests/ · /queue/minimax/h3-max/requests/ |

GET prefix + {request_id} retrieves results; GET prefix + {request_id}/status retrieves status. Fast/Mini share the version queue. When one path has multiple public aliases, add model to the creation body to select one; the gateway removes it before forwarding.

## Retries and settlement
Cancellation is not implemented and cancel_url is empty. Network errors and 5xx during creation are not automatically replayed, avoiding duplicate paid tasks; 429 follows routing policy. Forwarded webhooks do not trigger settlement: query terminal success with measured usage. Exact creation modes, fields and billing variants depend on deployment endpoint details; no arbitrary wildcard Fal proxy.

## Endpoint and authentication

~~~http
POST /queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video
~~~

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| prompt | string · required | Prompt; registered path selects the model. |
| duration / aspect_ratio | string | Valid values depend on operation. |

## Request example

~~~shell
curl "$XHUB_BASE_URL/queue/fal-ai/kling-video/v2.5-turbo/pro/text-to-video" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"A lighthouse at sunrise.","duration":"5"}'
~~~

## Success response

~~~json
{
  "request_id": "request-example"
}
~~~

| Field | Type |
| --- | --- |
| request_id | string |

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.


## Model guides

- [Seedance](fal-seedance)
- [Kling](fal-kling)
- [Vidu](fal-vidu)
- [Veo 3.1](fal-veo)
- [MiniMax H3 Max](fal-minimax)
