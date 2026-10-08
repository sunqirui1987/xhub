# XHub Customer Guide

**English** | [简体中文](user-guide.zh-CN.md) · [Documentation](README.md)

This guide takes a workspace from its first model to everyday administration. Install the services using the [installation guide](getting-started.md), then follow the steps below. Console paths refer to the web console, normally on port `3000`; application requests use the gateway, normally on port `4000`.

## 1. Set up your first workspace

Complete these steps with a platform administrator account:

1. Sign in at `/login`. The local Compose setup starts with `admin@xhub.local` / `admin-pass-1234`. Change the password in your profile before exposing the instance.
2. Open **Models + Endpoints** (`/models-and-endpoints`). Add a provider credential, then a model deployment. Check its public model name, upstream model ID, supported operation, API base, and price.
3. Create an organization at `/organizations`, then a team at `/teams`. Set the team's model scope and budget. The creator becomes its first team administrator unless another administrator is supplied.
4. Add accounts at `/users` with an email and a password of at least eight characters. Select their team and team role. To add an existing account, use its email in the team's member management.
5. Optionally create a project at `/projects` for an application or cost center.
6. Create a personal or service virtual key at `/api-keys`, within that team's scope.
7. Test an available model in `/playground`, make an application request, and confirm its usage at `/usage` and request details at `/logs`.

Platform administration and inference access are separate. A platform administrator also needs team membership to use personal inference access.

## 2. Connect models correctly

| Item | Meaning |
| --- | --- |
| Provider credential | The upstream connection: provider, API base, and provider secret |
| Model deployment | A model and supported operation attached to a credential |
| Public model name | The name applications send to XHub |
| Upstream model ID | The model identifier XHub sends to the provider |
| Price catalog | Pricing information; it does not create a deployment or grant access |

Several deployments can share a public model name. This gives routing a choice of upstreams without changing the application's model name.

A new instance may contain built-in FennoAI and Qiniu credential entries. Supply valid credentials and configure deployments before calling models. Validate the exact operation and parameters your application needs; a registered endpoint alone does not establish support for every upstream feature. A missing price does not mean a model is free.

## 3. Assign the right administrators

Account roles, organization roles, and team roles are separate assignments. A person can hold more than one.

| Role | Everyday responsibilities | Boundaries |
| --- | --- | --- |
| Platform administrator | Configure providers/models, create accounts and organizations, create/delete/move teams, set team models and budgets, block teams, inspect all usage/logs | Inference still requires its own authorized scope |
| Organization administrator | Manage its organization's name and administrators, team names/members/projects, organization usage/logs | Cannot create/delete teams, change team models/budgets, or read other people's personal secrets; this role alone does not grant service-key management |
| Team administrator | Manage its team's members/projects/service keys and inspect team usage/logs | Cannot change team model scope/budget or platform account roles |
| Member | Use permitted models and manage personal keys; inspect personal usage/logs | Cannot administer other members |

Only platform administrators create accounts. Account creation does not send an invitation email. Password recovery currently uses a direct administrator reset rather than an email reset link. A team administrator cannot reset a platform or organization administrator's password. Password changes invalidate older sessions, so the affected user must sign in again.

The last team administrator cannot be removed or demoted. Assign a replacement before changing that membership. Detailed endpoint rules are in the [developer permission reference](development/permissions.md).

## 4. Organize access and budgets

Resources follow **organization → team → project**. Use teams for a shared access boundary and projects for applications within that boundary.

- The team establishes the available model scope. An empty team model list means no restriction at that level.
- An empty project or key model list inherits its parent scope. A nonempty list narrows it and cannot grant models outside that parent.
- A project budget cannot exceed the team budget.
- To stop a team's access, block it. Clearing its model list does not disable it.

For example, a team allowed `chat-a` and `chat-b`, with a project narrowed to `chat-a`, can issue a project key for `chat-a`; that key cannot restore `chat-b`.

Budgets check recorded spend plus available hot spend. They do not reserve the cost of requests in advance: concurrent requests, long streams, and asynchronous tasks can overshoot a budget. Use conservative budgets and monitor provider charges as well.

## 5. Issue and manage virtual keys

| Key type | Recommended use | Management and departure behavior |
| --- | --- | --- |
| Personal key | An individual developer's scripts and experiments | Owned by that person within a team, optionally a project. Secret access is restricted to the owner and platform administrators. Removing the team membership revokes personal keys for that team. |
| Service key | A shared application, backend, or scheduled job | Owned by a team or project, managed by team/platform administrators. It remains active when a member leaves; rotate it if that member knew its secret. |

Give each application a recognizable key name. Set its team/project, allowed models, budget, RPM (requests per minute), TPM (tokens per minute), expiry, and route template as needed. Keep its secret in an environment variable or secret store.

Use XHub virtual keys in applications. Provider secrets connect XHub to upstreams; master credentials are for bootstrap/emergency administration; session tokens serve console sessions. Virtual keys do not grant console administration.

## 6. Make an application request

Install the OpenAI Python client with `pip install openai`. Set your virtual key and public model name:

```bash
export XHUB_API_KEY="YOUR_XHUB_VIRTUAL_KEY"
export XHUB_MODEL="YOUR_PUBLIC_MODEL_NAME"
```

```python
import os
from openai import OpenAI

client = OpenAI(
    api_key=os.environ["XHUB_API_KEY"],
    base_url="http://localhost:4000/v1",
)
response = client.chat.completions.create(
    model=os.environ["XHUB_MODEL"],
    messages=[{"role": "user", "content": "Hello, XHub!"}],
)
print(response.choices[0].message.content)
```

Replace the gateway URL for your deployment. A URL on port `3000` points to the console and can return HTML instead of an API response. For other operations, check the configured deployment's capability and use the corresponding supported API.

## 7. Configure routing

Manage templates at `/route-templates` with an account authorized for the target scope. Selection precedence is **key → team → organization → platform**. Clearing a selection restores inheritance. The selected template replaces the parent template as a whole; fields do not merge across scopes. Projects and accounts do not have a separate template assignment.

| Setting | Current behavior |
| --- | --- |
| `simple-shuffle` | Prefers the highest configured deployment weight; the name does not mean random shuffling |
| `weighted-split` | Smooth weighted round-robin; zero weight excludes a deployment. Affinity and cooldown can change observed ratios. |
| Cost selection | Compares input-token price, rather than estimating the full request bill |
| `num_retries` | Total attempts per deployment, including the first; missing or below one means one attempt. The console/global configuration can supply a different default. |
| Timeout | Per upstream attempt; default 60 seconds, rather than a deadline for the entire request |

Deployments of the same public model can fail over. Cross-public-model fallback is not implemented. Some advanced fields can be saved without having runtime behavior; consult the [routing reference](development/routing.md) before depending on them. Once a stream has emitted response bytes, XHub does not switch providers for that response.

## 8. Read usage, costs, and logs

Use `/usage` for trends and breakdowns and `/logs` for individual calls. Visibility follows the current account's role and scope. Keep the response's `x-litellm-call-id` when investigating a call. Platform administrators can read administrative audit records through `GET /audit`.

- Request prices are snapshotted so later price changes do not alter that request's recorded cost. Older records recalculated from current pricing are identified as such.
- Gateway response-cache hits have zero upstream cost. A provider's prompt-cache hit can still incur charges.
- Failed or interrupted requests can show zero local cost even if the provider charged them. Missing usage may be estimated. Missing prices require investigation.
- User, key, team, project, and organization totals describe the same calls from different dimensions. Adding them together double-counts spend.
- RPM/TPM use shared minute counters with Redis, or process-local sliding windows without Redis. For multiple gateway processes, configure shared runtime state.

Prompt/response storage is optional, controlled by `general_settings.store_prompts_in_spend_logs`. Scoped administrators can inspect stored content. Decide whether to enable it based on your data requirements and configure retention in your operations process; this setting does not provide automatic encryption or a retention policy.

The [pricing reference](development/pricing.md) explains rate units, snapshots, and settlement. The [runtime reference](development/runtime.md) describes cache, idempotency, streaming, and task boundaries.

## 9. Remove access safely

1. Assign another team administrator if necessary.
2. Remove the user's team membership. Personal keys for that team are revoked; historical usage remains attributed to the original call.
3. Revoke or rotate service keys whose secrets the departing user knew. Those keys do not belong to the member and survive departure.
4. Remove organization-administrator assignments separately. For full account offboarding, have a platform administrator disable the account.

## 10. Troubleshoot common problems

| Symptom | Check |
| --- | --- |
| No available models | Team membership, team/project/key model scope, configured deployments, and operation capability |
| `401` | Correct virtual key, expiry/revocation, disabled account, or an old session after a password reset |
| `403` or a hidden object's `404` | Role and resource scope, blocked team, model permission |
| `429` | Key RPM/TPM, budget status, and upstream rate limits |
| `502` / `503` | Upstream secret, API base/model ID, deployment health, database/Redis, route template, and call log |
| SDK receives HTML | Gateway URL should normally use port `4000`, not console port `3000` |
| Stream stops midway | Inspect the call log; decide whether the application can safely make a new request |
| Unexpected zero cost | Response cache, failed call, missing rate, or missing upstream usage; compare provider billing |
| Idempotency `409` | The same idempotency key was reused with changed raw request body or query |
| Cannot remove the last team administrator | Assign a replacement first |
| Changing the configured admin password has no effect | Configuration seeds a new account only; change/reset the existing account's password in the console |

For supported provider content-generation tasks, query with the same virtual key and provider route used to create the task. Task pins last seven days. XHub does not provide a universal media cancellation, callback, or download workflow.

## 11. Operate and upgrade the instance

Use the [installation guide](getting-started.md) for external URLs, environment secrets, and Docker configuration. Back up PostgreSQL and deployment configuration before upgrades; validate a restore in a separate database. The current identity schema has no automatic migration from the earlier schema.

For a code change or reproducible defect, include the affected operation, public model name, call ID, expected result, and relevant redacted logs. Developers can start with the [code and AI programming guide](development/README.md).
