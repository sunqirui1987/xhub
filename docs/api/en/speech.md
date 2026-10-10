# Speech generation

## Overview

Speech generation

### Speech generation

## Behavior
The actual response is binary audio, not this JSON illustration. Save it and inspect Content-Type.

## Endpoint and authentication

~~~http
POST /v1/audio/speech
~~~

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| model | string · required | Public model name; deployment must support this protocol. |
| input / voice | string · required | Text and supported voice ID. |
| response_format / speed | string / number | Output format and supported speech speed. |

## Request example

~~~shell
curl "$XHUB_BASE_URL/v1/audio/speech" \
  -H "Authorization: Bearer $XHUB_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"YOUR_MODEL_NAME","input":"Hello, welcome to XHub.","voice":"alloy"}' --output speech.mp3
~~~

## Success response

~~~json
{
  "note": "Binary audio response"
}
~~~

| Field | Type |
| --- | --- |
| note | string |

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.
