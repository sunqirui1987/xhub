# Seedance

## Overview

This registered video queue requires a deployment bound to its Fal transport. A Seedance model using content-generation transport cannot be called through Fal.

## Endpoint and authentication

~~~http
POST /queue/bytedance/seedance-2.0/text-to-video
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
curl "$XHUB_BASE_URL/queue/bytedance/seedance-2.0/text-to-video" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"A lighthouse at sunrise.","duration":5}'
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


## Task lifecycle

~~~shell
curl "$XHUB_BASE_URL/queue/bytedance/seedance-2.0/requests/$REQUEST_ID/status" -H "Authorization: Bearer $XHUB_API_KEY"
curl "$XHUB_BASE_URL/queue/bytedance/seedance-2.0/requests/$REQUEST_ID" -H "Authorization: Bearer $XHUB_API_KEY"
~~~

Save request_id as REQUEST_ID, poll status until completion, then retrieve the result. Prefer returned status_url and response_url resolved against the gateway root. Use the same XHub key. Cancellation is not implemented. Successful terminal results with measured usage settle once.

## Registered creation paths

| Family / variant | Path |
| --- | --- |
| bytedance 2.0 | POST /queue/bytedance/seedance-2.0/text-to-video |
| bytedance 2.0 | POST /queue/bytedance/seedance-2.0/image-to-video |
| bytedance 2.0 | POST /queue/bytedance/seedance-2.0/reference-to-video |
| bytedance 2.0/fast | POST /queue/bytedance/seedance-2.0/fast/text-to-video |
| bytedance 2.0/fast | POST /queue/bytedance/seedance-2.0/fast/image-to-video |
| bytedance 2.0/fast | POST /queue/bytedance/seedance-2.0/fast/reference-to-video |
| bytedance 2.0/mini | POST /queue/bytedance/seedance-2.0/mini/text-to-video |
| bytedance 2.0/mini | POST /queue/bytedance/seedance-2.0/mini/image-to-video |
| bytedance 2.0/mini | POST /queue/bytedance/seedance-2.0/mini/reference-to-video |
| bytedance 2.5 | POST /queue/bytedance/seedance-2.5/text-to-video |
| bytedance 2.5 | POST /queue/bytedance/seedance-2.5/image-to-video |
| bytedance 2.5 | POST /queue/bytedance/seedance-2.5/reference-to-video |
| byteplus 2.0 | POST /queue/byteplus/seedance-2.0/text-to-video |
| byteplus 2.0 | POST /queue/byteplus/seedance-2.0/image-to-video |
| byteplus 2.0 | POST /queue/byteplus/seedance-2.0/reference-to-video |
| byteplus 2.0/fast | POST /queue/byteplus/seedance-2.0/fast/text-to-video |
| byteplus 2.0/fast | POST /queue/byteplus/seedance-2.0/fast/image-to-video |
| byteplus 2.0/fast | POST /queue/byteplus/seedance-2.0/fast/reference-to-video |
| byteplus 2.0/mini | POST /queue/byteplus/seedance-2.0/mini/text-to-video |
| byteplus 2.0/mini | POST /queue/byteplus/seedance-2.0/mini/image-to-video |
| byteplus 2.0/mini | POST /queue/byteplus/seedance-2.0/mini/reference-to-video |
| byteplus 2.5 | POST /queue/byteplus/seedance-2.5/text-to-video |
| byteplus 2.5 | POST /queue/byteplus/seedance-2.5/image-to-video |
| byteplus 2.5 | POST /queue/byteplus/seedance-2.5/reference-to-video |

Image/reference fields follow deployment endpoint details. Fast/Mini share the version query queue; do not append /requests to a creation URL. For byteplus use /queue/byteplus/seedance-{version}/requests/{request_id}. Optional model selects a public alias and is removed before forwarding.
