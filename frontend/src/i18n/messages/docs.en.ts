import { docsHelpEn } from "./docs-help.en";
import { docsExpandedEn } from "./docs-expanded.en";
const familyArticles = Object.fromEntries(
  [
    ["fal-seedance", "Seedance"],
    ["fal-kling", "Kling"],
    ["fal-vidu", "Vidu"],
    ["fal-veo", "Veo 3.1"],
    ["fal-minimax", "MiniMax H3 Max"],
  ].map(([id, title]) => [id, { ...docsExpandedEn.fal, title }]),
);
const nativeArticles = {
  "native-anthropic": { ...docsExpandedEn["messages"], title: "Bypass Anthropic Messages" },
  "native-vertex": { ...docsExpandedEn["gemini"], title: "Bypass Vertex generateContent" },
  "native-gemini": { ...docsExpandedEn["gemini"], title: "Bypass Gemini generateContent" },
  "native-images": { ...docsExpandedEn["images"], title: "Bypass OpenAI image generation" },
  "native-image-edits": { ...docsExpandedEn["image-edits"], title: "Bypass OpenAI image edits" },
  "native-responses": { ...docsExpandedEn["responses"], title: "Bypass OpenAI Responses" },
  "native-chat": { ...docsExpandedEn["responses"], title: "Bypass OpenAI Chat Completions" },
};
const base = {
  center: "Documentation",
  search: "Search documentation",
  empty: "No matching documents. Try another keyword.",
  results: "{count} documents found",
  navigation: "Documentation categories",
  directory: "Documentation directory",
  onPage: "On this page",
  console: "Back to console",
  copy: "Copy example",
  copyCode: "Copy code",
  wrapCode: "Wrap lines",
  codePanel: "{language} code panel",
  copied: "Copied",
  copyFailed: "Copy failed. Select and copy the example manually.",
  example: "Request example",
  previous: "Previous",
  next: "Next",
  home: "Start here",
  note: "Examples use English prompts. Replace the gateway URL, API key and model name.",
  read: "Read documentation",
  groups: {
    product: "Product docs",
    tools: "Integrations",
    billing: "Billing",
    api: "API reference",
  },
  articles: {
    overview: {
      title: "Welcome to XHub",
      description: "One gateway for models and applications.",
      heading1: "Meet XHub",
      body1:
        "XHub provides a unified model gateway. Administrators connect providers and deploy models. Applications use public model names and virtual keys without receiving provider credentials.",
      heading2: "Make your first request",
      body2:
        "Follow the quickstart to configure providers, models and keys, then verify a response in the playground. Applications connect to the gateway service URL rather than a console page URL.",
      heading3: "Choose your guide",
      body3:
        "Product docs cover daily operations. Integrations provide SDK and client configuration. Billing explains charges and budgets. API reference covers requests and troubleshooting.",
    },
    quickstart: {
      title: "Quickstart",
      description: "Configure a model and make your first request.",
      heading1: "1. Connect a model",
      body1:
        "Add credentials on Model providers, then create a deployment in My models. Set its public name, upstream model, protocol and rates. Capabilities depend on deployment and provider support.",
      heading2: "2. Create a key",
      body2:
        "Create a virtual key on API keys and set model scope and budget. Store it in the server environment variable XHUB_API_KEY. Set XHUB_BASE_URL to the gateway root without /v1. Keep keys out of source control.",
      heading3: "3. Request and verify",
      body3:
        "Replace YOUR_MODEL_NAME with your public model name. After success, inspect status, tokens and cost in request logs by call ID. For failures, check model permissions and protocol.",
    },
    keys: {
      title: "API keys and permissions",
      description: "Manage application identity, access scope and budgets.",
      heading1: "Create and store",
      body1:
        "Create and securely store a dedicated virtual key per application. Provider credentials authenticate upstream calls; virtual keys authenticate applications to XHub. They are separate credentials.",
      heading2: "Permissions and ownership",
      body2:
        "Keys can belong to users, teams or projects. Roles, ownership and configuration constrain visibility and allowed models. A visible model does not imply support for all protocols.",
      heading3: "Revoke and troubleshoot",
      body3:
        "Delete unused keys. For 401, check credentials. For 403, check permissions. For budget errors, inspect key and related entity limits.",
    },
    routing: {
      title: "Routing and guardrails",
      description: "Configure model access, retries and request protection.",
      heading1: "Models and deployments",
      body1:
        "Applications send a public model name; the gateway selects an available deployment. Multiple deployments may serve one public name. Upstream and public names may differ.",
      heading2: "Route templates",
      body2:
        "Configure weights, retries and timeouts in route templates and bind them to the intended scope. Parameters must fit deployment protocols. Verify in the playground first.",
      heading3: "Guardrails and logs",
      body3:
        "Guardrails block, redact or allow according to rules and execution stage. Inspect rules, responses and logs to distinguish policy decisions from provider failures.",
    },
    toolsOverview: {
      title: "Integration overview",
      description: "Connect the OpenAI SDK and compatible clients.",
      heading1: "Prepare connection details",
      body1:
        "Prepare the gateway URL, XHub virtual key and public model name. Use the same gateway root Base URL as the playground; do not append /v1. Field names depend on tool version.",
      heading2: "Choose an integration",
      body2:
        "Python applications can use the SDK example. Tools supporting custom OpenAI endpoints can use client configuration. Tools restricted to fixed provider URLs cannot directly use a custom gateway.",
      heading3: "Verify compatibility",
      body3:
        "List models and send a minimal chat request. Responses, images, audio and streaming require matching deployment protocols. Connection success does not prove every feature is supported.",
    },
    sdk: {
      title: "OpenAI SDK",
      description: "Configure the URL and key in Python applications.",
      heading1: "Install and configure",
      body1:
        "Install the openai package and set XHUB_BASE_URL and XHUB_API_KEY. The root URL excludes /v1; the SDK appends its endpoint path.",
      heading2: "Send a chat request",
      body2:
        "Replace the public model name in the example. It uses a non-streaming request. For streaming, set stream=True and iterate over events.",
      heading3: "Handle errors",
      body3:
        "Handle authentication, permission, rate limit and connection errors. Set timeouts and bounded retries. Retries may add calls and charges; apply application idempotency rules.",
    },
    clients: {
      title: "Clients and coding tools",
      description: "Configure tools supporting custom endpoints.",
      heading1: "Connection settings",
      body1:
        "Choose an OpenAI compatible service. Set Base URL to the same gateway root URL used by the playground, API Key to the virtual key and model to the public model name.",
      heading2: "Environment variable example",
      body2:
        "The example shows common environment conventions. Some tools use settings panels or other names. Check tool documentation and replace placeholders.",
      heading3: "Capability boundaries",
      body3:
        "Compatibility with every tool version is not guaranteed. Responses and provider specific protocols require matching deployments. Use the SDK or another tool if endpoints cannot be changed or protocols are unsupported.",
    },
    usage: {
      title: "Usage based billing",
      description: "Understand usage, rates and historical charges.",
      heading1: "Charge calculation",
      body1:
        "Cost equals measured quantity times the USD rate per base unit. Input, output and cache use applicable rates. Images, seconds and queries use their own units. Example: 1,000 input tokens × $0.000002 + 500 output tokens × $0.000008 = $0.006.",
      heading2: "Rates and windows",
      body2:
        "Deployment rates take precedence over catalog rates. Missing prices do not mean free usage. Peak and off-peak rates use call start time in Asia/Shanghai.",
      heading3: "Logs and reconciliation",
      body3:
        "Request logs show usage, charges and price snapshots. Snapshots preserve applied rates; rate changes do not rewrite history. Reconcile local costs with provider bills. Paginated logs and scan-limited reports are not complete invoices.",
    },
    budgets: {
      title: "Budgets and limits",
      description: "Control budgets and request rates.",
      heading1: "Configure budgets",
      body1:
        "Allocate Organization → Team → Person → API Key budgets in USD. Spending plus unused child allocations must fit within the parent. Blank shares the parent remainder; zero blocks spending. Budgets are cumulative, without monthly resets. Each person belongs to at most one team. After leaving, personal budgets remain independent. Unrestricted business uses a separate unlimited organization and team.",
      heading2: "Request limits",
      body2:
        "Set RPM and TPM on organization, team, user and API Key pages. Fixed child allocations sum within each parent; blank intermediates pass fixed descendants upward. Blank shares unreserved capacity and zero blocks calls. UTC calendar minutes reset counters; admission atomically counts the unique path and 429 consumes no capacity. TPM uses the incoming estimate rather than final output usage. Multiple instances need shared Redis.",
      heading3: "Handle rejections",
      body3:
        "Inspect key and related entity settings, then logs. Changes require management permissions. Displayed balances are not provider invoices or guarantees about concurrent settlement.",
    },
    models: {
      title: "List models",
      description: "GET /models: discover enabled public models without a key.",
      heading1: "Authentication and path",
      body1:
        "GET /models requires no key and lists enabled public model names. With an explicit key, the list is filtered by its permissions. GET /v1/models remains authenticated; inference always requires a valid key.",
      heading2: "Response and selection",
      body2:
        "The data array lists models. Use an id as model in requests. Caller permissions constrain the list; verify capabilities against deployment protocols.",
      heading3: "Permissions and failures",
      body3:
        "Invalid keys return authentication errors. Unauthorized calls return permission errors or hide resource existence. For missing models, check scope, deployment status and public names.",
    },
    chat: {
      title: "Chat completions",
      description: "POST /v1/chat/completions: send messages.",
      heading1: "Request fields",
      body1:
        "model is the public name; messages contains role and content objects. Use JSON and Bearer authentication. stream defaults to non-streaming. Optional parameters require deployment and upstream support.",
      heading2: "Responses and streaming",
      body2:
        "Read choices and usage for non-streaming responses. stream=true uses SSE until completion. Errors after streaming starts cannot change HTTP status; inspect events and logs.",
      heading3: "Request tracing",
      body3:
        "Use x-litellm-call-id to find logs. x-litellm-response-cost is the locally calculated USD charge, not a persistence commit receipt. Inspect log snapshots for details.",
    },
    errors: {
      title: "Errors and troubleshooting",
      description: "Diagnose issues by status code and call ID.",
      heading1: "Request and permission errors",
      body1:
        "For 400, inspect JSON, model and parameters. For 401, check credentials. For 403, check role, model scope and ownership. Some unauthorized calls return 404 to hide existence.",
      heading2: "Rate limits and upstream failures",
      body2:
        "For 429, inspect Retry-After and rate settings. For 5xx, check dependencies, provider connectivity, deployments and logs. Use backoff and bounded retries.",
      heading3: "Keep diagnostic details",
      body3:
        "Record time, model, status and call ID. Remove keys, personal information and sensitive content before sharing. Also inspect streaming event errors.",
    },
  },
};

export const docsEn = {
  ...base,
  ...{
    required: "required",
    status200: "Success example. Fields may vary by protocol and provider.",
    status400: "Invalid request. Check JSON, parameters and query.",
    status401: "Missing or invalid XHub credential.",
    status403: "Permission denied. Check model scope or policy.",
    status429: "Limit exceeded. Inspect response details and Retry-After.",
  },
  ...{
    parameters: "Parameters",
    runner: {
      close: "Close runner",
      method: "HTTP method",
      title: "Run online",
      hint: "Sends a real request to the current gateway and may incur usage and costs. The key stays in page memory. Enter an authorized public model name; requests are not automatically retried.",
      key: "XHub API key",
      path: "Request path",
      body: "Request body (JSON)",
      multipart: "Use multipart file upload",
      fileField: "File field name",
      file: "Upload file",
      run: "Run request",
      running: "Running…",
      invalid: "Enter a key and a valid path within the gateway.",
      jsonError: "The request body must be a JSON object.",
      networkError: "Request failed or timed out. Check the gateway connection and browser cross-origin configuration.",
      binary: "HTTP {status}, binary response {type}, {size} bytes.",
    },
    protocols: {
      bypass: "Native protocol Bypass",
      fal: "Fal · asynchronous queue",
      contents: "Content generation · Seedance",
      openai: "OpenAI compatible",
      native: "Native protocols",
      general: "Protocol reference",
    },
    sections: {
      bypass: "Native protocol Bypass",
      help: "Help and troubleshooting",
      openai: "OpenAI compatible protocol",
      native: "Native protocols and Bypass",
      tasks: "Asynchronous task protocols",
      start: "Getting started",
      management: "Configuration and management",
      sdks: "SDKs",
      coding: "Coding tools",
      clients: "Chat clients",
      agents: "Agent frameworks",
      costs: "Cost calculation",
      limits: "Budgets and limits",
      models: "Models",
      text: "Text and conversation",
      vectors: "Vectors and retrieval",
      safety: "Moderation",
      images: "Image generation and editing",
      audio: "Audio",
      video: "Video and asynchronous tasks",
      protocols: "Protocols and errors",
    },
    gatewayBase:
      "Current gateway Base URL: {base}. Matches the playground; do not append /v1 to the root. curl uses the complete endpoint path below.",
    field: "Field",
    type: "Type",
    description: "Description",
    response: "Response",
    auth: "Send Authorization: Bearer <XHUB_API_KEY>. Examples are illustrative; optional fields require upstream support.",
  },
  articles: Object.fromEntries(
    Object.entries({ ...base.articles, ...docsHelpEn, ...docsExpandedEn, ...familyArticles, ...nativeArticles }).map(
      ([id, article]) => [id, { ...base.articles[id as keyof typeof base.articles], ...article }],
    ),
  ),
};
