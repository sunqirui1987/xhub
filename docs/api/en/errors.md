# Errors and troubleshooting

## Overview

Diagnose issues by status code and call ID.

### Request and permission errors

For 400, inspect JSON, model and parameters. For 401, check credentials. For 403, check role, model scope and ownership. Some unauthorized calls return 404 to hide existence.

### Rate limits and upstream failures

For 429, inspect Retry-After and rate settings. For 5xx, check dependencies, provider connectivity, deployments and logs. Use backoff and bounded retries.

### Keep diagnostic details

Record time, model, status and call ID. Remove keys, personal information and sensitive content before sharing. Also inspect streaming event errors.

## Endpoint and authentication

Use an XHub API key: Authorization: Bearer <XHUB_API_KEY>. Do not use upstream provider credentials as gateway keys.

Use the gateway root Base URL shown in the playground, without appending /v1; send requests to the full endpoint path above.

## Request parameters

This is a protocol guide. Use the corresponding endpoint article for its parameters.

## Request example

See the executable example in the corresponding endpoint article.

## Success response

Responses follow the selected native protocol and depend on the endpoint and upstream model.

## Errors and troubleshooting

| HTTP | Meaning |
| --- | --- |
| 400 | Invalid request. Check JSON, parameters and query. |
| 401 | Missing or invalid XHub credential. |
| 403 | Permission denied. Check model scope or policy. |
| 429 | Limit exceeded. Inspect response details and Retry-After. |

## Capabilities and billing boundaries

Parameter support depends on the deployed model and conversion or Bypass path; upstream extension fields are not universally supported. Use the implemented protocol adapter and upstream response as the contract. Usage details are subsets of input/output totals and must not be added twice; reconcile costs against the billing snapshot in gateway logs. Example responses illustrate fields only.
