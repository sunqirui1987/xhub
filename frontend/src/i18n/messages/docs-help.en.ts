export const docsHelpEn = {
  "help-setup": {
    title: "Installation and first configuration",
    description: "From deployment to your first request",
    heading1: "From deployment to your first request",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "Follow the repository installation guide to deploy the gateway, console and storage. Check both console and gateway access. Sign in, add provider credentials, then create a deployment. An empty model catalog normally means no deployment has been created.",
    body2:
      "Confirm the public model name and supported endpoints in the catalog. Test an English prompt in Playground. Once successful, create a minimally scoped XHub virtual key for your application. Keep provider credentials private.",
    body3:
      "Use the Base URL shown in Playground, keeping any reverse proxy prefix and without appending /v1. Requests use the complete endpoint path for the chosen protocol. Back up the database and deployment configuration before upgrading.",
  },
  "help-models": {
    title: "Providers and model deployments",
    description: "Connect usable models",
    heading1: "Connect usable models",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "Choose the actual protocol and credential type on the providers page. Configure the upstream address and credentials. Set a public model name, upstream model identifier and provider in deployments. Clients send the public model name; the deployment maps it upstream.",
    body2:
      "A public model may have multiple deployments. Configure actual capabilities, pricing and enabled state for each deployment, then test it in diagnostics or Playground. A provider name alone does not establish endpoint support.",
    body3:
      "For an empty model list, check enabled deployments and the current user or key model scope. For unsupported protocol errors, compare the client endpoint with the deployment transport.",
  },
  "help-workspace": {
    title: "Organizations, teams and access",
    description: "Establish access boundaries",
    heading1: "Establish access boundaries",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "Create organizations and teams, add members and assign roles. Configure team model access and budgets; use projects for business attribution. Grant only the management permissions members need.",
    body2:
      "When creating personal or service keys, check team ownership, model scope, expiration and budget. Model scope cannot exceed parent permissions. See billing and quota guides for allocated limits, shared balances and unlimited settings.",
    body3:
      "On departure, revoke related keys, remove membership and check whether services still use personal credentials. Preserve historical usage for auditing.",
  },
  "help-playground": {
    title: "Playground and application integration",
    description: "Test before integrating",
    heading1: "Test before integrating",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "Select an authorized public model and matching endpoint in Playground, then send a prompt or protocol body. Streaming, multimodal and file parameters depend on deployment capability.",
    body2:
      "After a successful test, copy the request example and store your application key in an environment variable. Use the same gateway Base URL. The tools category covers coding tools, chat clients and SDK setup.",
    body3:
      "API pages can run requests directly: enter a key, path and JSON, then inspect the HTTP status and response. Use multipart for files; select GET and the task path for polling. Real requests may incur costs and are not retried automatically.",
  },
  "help-routing": {
    title: "Routing and fallback",
    description: "Control deployment selection",
    heading1: "Control deployment selection",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "Add usable deployments for a public model before selecting a routing strategy or template. Choose weights, load, latency or cost according to business needs. Confirm candidate deployments support the request.",
    body2:
      "Routing templates can inherit through key, team and organization scopes. Clearing a binding restores inheritance. Templates choose deployments within existing permissions and do not expand model access.",
    body3:
      "For duplicate tasks or unexpected costs, inspect creation retry policies. Avoid resending media creation requests with unknown outcomes. Use deployment, error type and call ID in logs to trace selection.",
  },
  "help-guardrails": {
    title: "Guardrail configuration and validation",
    description: "Inspect, block and redact",
    heading1: "Inspect, block and redact",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "Configure built-in rules or custom XGo checks on the guardrails page. Save a draft and test normal and violating inputs. Current rules cover pre-call text checks; do not assume every media input or output is scanned.",
    body2:
      "Check execution stage, scope and action before enabling a rule. Blocking stops upstream execution; redaction sends modified content; warning records the event. Verify bindings with a real application request.",
    body3:
      "When external moderation is unavailable, check adapter configuration and failure policy, then review guardrail logs. Keep real credentials and personal data out of public issue reports.",
  },
  "help-logs": {
    title: "Usage, costs and call diagnostics",
    description: "Review a request using evidence",
    heading1: "Review a request using evidence",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "Inspect model, deployment, status, latency and call ID in call logs. Review usage and the recorded price snapshot in cost details. Do not recompute historical requests with current prices.",
    body2:
      "Filter costs by user, key, team or project and check attribution and time range. Cached input is part of input usage. Images, audio and video use their own pricing units; see billing formulas and examples.",
    body3:
      "Poll asynchronous tasks with the same task identity. Completed task costs may appear after creation. Preserve task ID, call ID, status and pricing snapshot when investigating discrepancies; logs are not a complete provider invoice.",
  },
  "help-troubleshooting": {
    title: "Common problems and troubleshooting",
    description: "Diagnose using the response",
    heading1: "Diagnose using the response",
    heading2: "Steps",
    heading3: "Checks and limits",
    body1:
      "401: check that the key is an active XHub virtual key. 403: inspect model scope, team membership and role. 400: check JSON, required fields, public model name and endpoint protocol; preserve the response body.",
    body2:
      "429: inspect RPM, TPM, quota and upstream rate limits. 5xx: use the call ID to locate deployment, upstream errors and timeouts. If curl works but browser requests fail, check gateway URL, HTTPS mixed content and CORS origins.",
    body3:
      "Empty model list: check enabled deployments and key authorization. Tool connection failure: use Playground Base URL without appending /v1 and inspect paths added by the tool. Media failures: distinguish creation, polling and download, and check implemented capabilities.",
  },
};
