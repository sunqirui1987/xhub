# List models

## Overview

GET /models: discover enabled public models without a key.

### Authentication and path

GET /models requires no key and lists enabled public model names. With an explicit key, the list is filtered by its permissions. GET /v1/models remains authenticated; inference always requires a valid key.

### Response and selection

The data array lists models. Use an id as model in requests. Caller permissions constrain the list; verify capabilities against deployment protocols.

### Permissions and failures

Invalid keys return authentication errors. Unauthorized calls return permission errors or hide resource existence. For missing models, check scope, deployment status and public names.

## Protocol behavior
GET /models allows anonymous discovery of enabled public model names. Explicit credentials retain permission filtering and invalid keys return 401. GET /v1/models remains authenticated. id is callable; created is a fixed compatibility timestamp and owned_by a compatibility label, not the real provider. Empty data can mean no granted models.

## Endpoint and authentication

~~~http
GET /models
~~~

GET /models publicly lists enabled model names without credentials. Explicit credentials filter access; invalid credentials return 401. GET /v1/models retains authenticated compatibility.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

| Field | Type | Description |
| --- | --- | --- |
| scope | string | Empty or expand; other values return 400. expand broadens administrator visibility. |
| team_id | string | Team filter for sessions; keys keep their bound scope. |
| return_wildcard_routes | boolean | Include permitted wildcard names; default false. |
| only_model_access_groups | boolean | Legacy flag; true returns an empty list. |

## Request example

~~~shell
curl "$XHUB_BASE_URL/models"
~~~

## Success response

~~~json
{
  "object": "list",
  "data": [
    {
      "id": "YOUR_MODEL_NAME",
      "object": "model",
      "created": 1677610602,
      "owned_by": "openai"
    }
  ]
}
~~~

| Field | Type |
| --- | --- |
| object | string |
| data | object[] |
| data[].id | string |
| data[].object | string |
| data[].created | number |
| data[].owned_by | string |

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.
