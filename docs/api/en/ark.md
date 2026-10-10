# Content generation task protocol

## Overview

Content generation task protocol

### Content generation task protocol

## Protocol and applicability
This page describes a content-generation task protocol; vendors are deployment implementations. The standard /api/v3 variant supports POST/GET collection and GET /api/v3/contents/generations/tasks/{id}. The /v3 deployment variant supports POST /v3/contents/generations/tasks and GET /v3/contents/generations/tasks/{id}. Routing context is retained seven days. No generic callback or cancellation. Terminal success with measured usage settles once.

## Endpoint and authentication

~~~http
POST /api/v3/contents/generations/tasks
~~~

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| model | string · required | Public model alias bound to the matching content-generation transport. |
| content | object[] · required | Text and media input parts. |
| content[].type | string | text, image_url, video_url or audio_url as supported by the upstream model. |
| content[].text | string | Required for text parts; example prompts use English. |
| content[].image_url.url | string | Image reference URL; availability and accepted formats follow upstream rules. |
| content[].video_url.url / audio_url.url | string | Video/audio reference URL when supported by the selected model. |
| content[].role | string | Media purpose, for example first_frame, last_frame, reference_image, reference_video, reference_audio. Do not assume every model supports every combination. |
| duration | integer | Requested seconds; allowed range and automatic-duration semantics depend on model/version. Not measured billable usage. |
| resolution / ratio | string | Resolution/aspect ratio supported by deployment; no universal version-independent range. |
| generate_audio | boolean | Request synchronized audio if supported. |
| return_last_frame | boolean | Request last-frame output if supported. |
| seed / watermark | integer / boolean | Optional upstream generation controls, subject to selected model. |

## Request example

~~~shell
curl "$XHUB_BASE_URL/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","content":[{"type":"text","text":"A lighthouse at sunrise."}]}'
~~~

## Success response

~~~json
{
  "id": "task-example"
}
~~~

| Field | Type |
| --- | --- |
| id | string |

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.

## Task lifecycle and protocol choice

Save the creation id as TASK_ID. Query until a terminal state. Content-generation transport uses content arrays. See [Seedance Fal](fal-seedance) for deployments bound to Fal; the protocols are not interchangeable.

~~~http
GET /api/v3/contents/generations/tasks/{TASK_ID}
GET /v3/contents/generations/tasks/{TASK_ID}
~~~

## Query response fields

| Field | Meaning |
| --- | --- |
| id | Task identifier returned during creation. |
| status | Upstream state; commonly queued, running, succeeded, failed, expired. HTTP 200 alone does not mean generation succeeded. |
| content.video_url | Completed video URL when available; download within upstream retention. |
| content.last_frame_url | Last-frame URL if requested and supported. |
| error.code / error.message | Inspect these for failed task details. |
| usage.completion_tokens / total_tokens | Measured usage for settlement; total is not an extra quantity to add again. |
| created_at / updated_at | Upstream creation/update timestamps if returned. |

~~~json
{
  "id": "task-example",
  "status": "succeeded",
  "content": {
    "video_url": "https://example.com/output.mp4"
  },
  "usage": {
    "completion_tokens": 1000,
    "total_tokens": 1000
  }
}
~~~

Example only. XHub does not implement Modelink asset creation, virtual-human review, qvideo task conversion, cancellation or callback settlement. Native upstream extensions may be passed through, but support depends on the deployed upstream contract.
