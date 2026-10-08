export const routeTemplatesGuideEn = {
  tab: "Configuration guide",
  title: "Complete route-template JSON guide",
  intro:
    "A template is one complete router-settings object. Use the forms for common settings. Use the JSON tab for bulk import and advanced fields.",
  ruleTitle: "A template applies as one document",
  ruleBody:
    "When an organization, team, or key selects a template, this entire document applies. Fields are not merged with a parent template. New templates start as a copy of the platform default so important values are retained.",
  currentStatusTitle: "Check which fields the current gateway executes",
  currentStatusBody:
    "The request path currently executes routing_strategy, weighted-split, num_retries, timeout, allowed_fails, and cooldown_time. Fallback chains, per-error retries, routing groups, aliases, pre-call checks, tag filtering, stream timeout, and retry delay are stored and exported but do not yet affect requests. Their complete JSON formats remain documented below for migration and future enablement.",
  flowTitle: "How one request is processed",
  flowHint: "These four stages distinguish load balancing, retries, cooldown, and cross-model fallback.",
  flow1Title: "Choose a deployment of the current model",
  flow1Body:
    "routing_strategy selects one deployment under the same public model name. weighted-split uses weights; a routing group can set a separate strategy for a set of models.",
  flow2Title: "Retry the current deployment",
  flow2Body:
    "num_retries currently controls attempts on one deployment, and timeout limits each upstream call. retry_policy and retry_after are compatibility settings that the current request path does not execute.",
  flow3Title: "Mark an unhealthy deployment",
  flow3Body:
    "After a deployment accumulates allowed_fails failures, it enters cooldown for cooldown_time and is excluded from routing during that period.",
  flow4Title: "Move to a fallback model",
  flow4Body:
    "The designed order reads a matching fallback chain after deployments and retries are exhausted, counting each model switch once. The current gateway stores this configuration but does not execute this stage yet.",
  fallbackRecipes: "Fallback recipes",
  normalFallbackTitle: "Normal failures: switch models in order",
  normalFallbackBody:
    "The intended behavior moves from gpt-main to claude-main, then gemini-main after connection failures, 429, 5xx, or allowed timeouts. The current version only stores this configuration.",
  contextFallbackTitle: "Context overflow: use a long-context model",
  contextFallbackBody:
    "The intended behavior uses only context_window_fallbacks after a pre-call context overflow check. The current version only stores this configuration.",
  safeFallbackTitle: "Avoid another request after a timeout",
  safeFallbackBody:
    "Once fallback execution is enabled, removing ambiguous prevents a timeout or post-send disconnect from switching models and reduces duplicate-request risk.",
  referenceTitle: "Field reference",
  referenceHint:
    "Defaults describe baseline behavior when the template omits a field. Platform default means the current deployment-level value.",
  field: "Field",
  type: "Type",
  defaultValue: "Default",
  support: "Current status",
  active: "Active",
  stored: "Stored only",
  purpose: "Purpose and notes",
  example: "Example",
  platformDefault: "Platform default",
  balanceFields: "Load balancing and routing groups",
  balanceFieldsHint: "Chooses a deployment before any cross-model fallback happens.",
  retryFields: "Retries and timeouts",
  retryFieldsHint: "Controls repeated attempts on the current deployment before cross-model fallback.",
  fallbackFields: "Cross-model fallbacks",
  fallbackFieldsHint: "Chooses another public model name when the current model cannot complete a request.",
  healthFields: "Deployment health and name mapping",
  healthFieldsHint: "Controls when failed deployments pause routing and how caller-visible names map.",
  fieldRoutingStrategy:
    "Selection strategy among deployments of one public name. Values include simple-shuffle, least-busy, latency-based-routing, cost-based-routing, usage-based-routing-v2, and weighted-split.",
  fieldRoutingStrategyArgs:
    "The current request path reads weights for weighted-split. Other strategy arguments are stored but are not currently read by that path.",
  fieldRoutingGroups:
    "Named routing groups. Each item has group_name, models, routing_strategy, and optional routing_strategy_args. Prefer editing these on the Routing groups tab.",
  fieldTagFiltering: "Intended to select only deployments whose tags match the request; currently stored only.",
  fieldNumRetries:
    "Total attempts on the current deployment. This gateway treats 1 as one attempt and currently executes this field.",
  fieldRetryPolicy:
    "Compatibility object for retry counts by error type. It can contain BadRequestErrorRetries, AuthenticationErrorRetries, TimeoutErrorRetries, RateLimitErrorRetries, ContentPolicyViolationErrorRetries, and InternalServerErrorRetries; currently stored only.",
  fieldGroupRetryPolicy:
    "Overrides retry_policy for one public model name. The first-level key is the model name; the nested object contains error types and counts. Set it in Other fields or on the JSON tab.",
  fieldRetryAfter: "Intended to set the minimum wait before another attempt; currently stored only.",
  fieldTimeout: "Timeout in seconds for one upstream call. Each retry starts a new call.",
  fieldStreamTimeout:
    "Intended to set a separate streaming timeout; currently stored only, while requests still use timeout.",
  fieldFallbacks:
    "Normal fallback chains. Each item maps a primary model to an ordered target list; currently stored only and does not switch models.",
  fieldContextFallbacks:
    "Dedicated chain for context-window overflow. It does not continue into normal fallbacks; enable_pre_call_checks is usually enabled with it.",
  fieldContentFallbacks: "Dedicated chain for content-policy rejection. It does not continue into normal fallbacks.",
  fieldDefaultFallbacks: "Ordered general list used when the current model has no dedicated chain in fallbacks.",
  fieldFallbackCauses:
    "Failure classes allowed to start a normal fallback: no_response means no response arrived, status means 429/5xx, and ambiguous means a timeout or unknown upstream execution state. Remove ambiguous to reduce duplicate-request risk.",
  fieldMaxFallbacks: "Maximum number of other models one request may enter. Each model switch counts once.",
  fieldPreCallChecks: "Checks context size before sending the request; context-window fallback depends on it.",
  fieldAllowedFails:
    "Number of accumulated deployment failures before cooldown. A value below 1 disables failure recording and cooldown.",
  fieldCooldown:
    "Seconds a failed deployment is excluded from routing. Under the current contract, 0 means 60 seconds.",
  fieldAlias:
    "Intended to map caller-visible aliases to existing public model names; currently stored only. Keys are aliases and values are targets.",
  fullExample: "Complete JSON example",
  fullExampleHint:
    "This example includes load balancing, a routing group, retries, all three fallback types, cooldown, and an alias. Replace the model names and URLs, then paste it into the JSON tab.",
  copy: "Copy example",
  copied: "Copied",
} as const;
