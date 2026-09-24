import { mapDisplayToInternalNames } from "../callback_info_helpers";
import { NEVER_RESETS_BUDGET_DURATION } from "../common_components/budget_duration_dropdown";
import type { RouterSettingsAccordionValue } from "../common_components/RouterSettingsAccordion";
import type { BudgetWindowEntry } from "../key_team_helpers/BudgetWindowsEditor";
import type { ModelMaxBudget } from "../key_team_helpers/ModelMaxBudgetEditor";
import { tagRowsToLimits, type TagRateLimitEntry } from "../key_team_helpers/TagRateLimitEditor";

export interface KeyLoggingSetting {
  callback_name?: string;
}

export interface ExistingKey {
  readonly team_id?: string | null;
  readonly key_alias?: string | null;
}

export interface KeyCreateInput {
  readonly formValues: Record<string, unknown>;
  readonly existingKeys: readonly ExistingKey[] | null;
  readonly keyOwner: string;
  readonly userID: string | null;
  readonly selectedAgentId: string | null;
  readonly loggingSettings: KeyLoggingSetting[];
  readonly disabledCallbacks: string[];
  readonly autoRotationEnabled: boolean;
  readonly rotationInterval: string;
  readonly modelAliases: Record<string, string>;
  readonly routerSettings: RouterSettingsAccordionValue | null;
  readonly budgetLimits: BudgetWindowEntry[];
  readonly tagRateLimits: TagRateLimitEntry[];
  readonly budgetFallbacks: Record<string, string[]>;
  readonly modelMaxBudget: ModelMaxBudget;
}

export type KeyPayloadResult =
  | {
      readonly kind: "ok";
      readonly payload: Record<string, unknown>;
      readonly endpoint: "standard" | "service_account";
    }
  | { readonly kind: "duplicate_alias"; readonly alias: string; readonly teamId: string | null }
  | { readonly kind: "agent_not_selected" };

const parseMetadata = (raw: unknown): unknown => {
  try {
    return JSON.parse((raw as string) || "{}");
  } catch (error) {
    console.error("Error parsing metadata:", error);
    return {};
  }
};

const buildMetadataJson = (values: Record<string, unknown>, input: KeyCreateInput): string => {
  const parsed = parseMetadata(values.metadata);
  if (input.keyOwner === "service_account") {
    (parsed as Record<string, unknown>).service_account_id = values.key_alias;
  }
  const logged =
    input.loggingSettings.length > 0
      ? { ...(parsed as object), logging: input.loggingSettings.filter((config) => config.callback_name) }
      : parsed;
  const disabled =
    input.disabledCallbacks.length > 0
      ? { ...(logged as object), litellm_disabled_callbacks: mapDisplayToInternalNames(input.disabledCallbacks) }
      : logged;
  return JSON.stringify(disabled);
};

const droppedColumnKeys = new Set<string>([
  "allowed_vector_store_ids",
  "allowed_mcp_servers_and_groups",
  "allowed_mcp_access_groups",
  "mcp_tool_permissions",
  "allowed_agents_and_groups",
  "allowed_skills",
  "policies",
  "prompts",
  "object_permission",
]);

const consumedSourceKeys = (values: Record<string, unknown>): ReadonlySet<string> =>
  new Set<string>([
    ...droppedColumnKeys,
    ...(values.disable_global_guardrails ? [] : ["disable_global_guardrails"]),
  ]);

const withoutKeys = (values: Record<string, unknown>, dropped: ReadonlySet<string>): Record<string, unknown> =>
  Object.fromEntries(Object.entries(values).filter(([key]) => !dropped.has(key)));

const duplicateAlias = (input: KeyCreateInput): { alias: string; teamId: string | null } | undefined => {
  const alias = (input.formValues?.key_alias as string | undefined) ?? "";
  const teamId = (input.formValues?.team_id as string | undefined) ?? null;
  const taken = (input.existingKeys ?? []).filter((key) => key.team_id === teamId).map((key) => key.key_alias);
  return taken.includes(alias) ? { alias, teamId } : undefined;
};

export const buildKeyCreatePayload = (input: KeyCreateInput): KeyPayloadResult => {
  const duplicate = duplicateAlias(input);
  if (duplicate) {
    return { kind: "duplicate_alias", ...duplicate };
  }
  if (input.keyOwner === "agent" && !input.selectedAgentId) {
    return { kind: "agent_not_selected" };
  }

  const values = input.formValues;

  const dropped = consumedSourceKeys(values);

  const duration = values.duration;
  const validWindows = input.budgetLimits.filter(
    (window) => window.budget_duration && window.max_budget !== null && window.max_budget !== undefined,
  );
  const { tag_rpm_limit } = tagRowsToLimits(input.tagRateLimits);
  const routerSettings = input.routerSettings?.router_settings;
  const configuredRouterSettings =
    routerSettings &&
    Object.values(routerSettings).some((value) => value !== null && value !== undefined && value !== "")
      ? routerSettings
      : undefined;

  return {
    kind: "ok",
    endpoint: input.keyOwner === "service_account" ? "service_account" : "standard",
    payload: {
      ...withoutKeys(values, dropped),
      ...(values.organization_id === null && { organization_id: undefined }),
      ...(values.project_id === null && { project_id: undefined }),
      ...(input.keyOwner === "you" && { user_id: input.userID }),
      ...(input.autoRotationEnabled && { auto_rotate: true, rotation_interval: input.rotationInterval }),
      duration: !duration || (duration as string).trim() === "" ? null : duration,
      metadata: buildMetadataJson(values, input),
      ...(Object.keys(input.modelAliases).length > 0 && { aliases: JSON.stringify(input.modelAliases) }),
      ...(configuredRouterSettings && { router_settings: configuredRouterSettings }),
      ...(validWindows.length > 0 && { budget_limits: validWindows }),
      ...(Object.keys(tag_rpm_limit).length > 0 && { tag_rpm_limit }),
      ...(Object.keys(input.budgetFallbacks).length > 0 && { budget_fallbacks: input.budgetFallbacks }),
      ...(Object.keys(input.modelMaxBudget).length > 0 && { model_max_budget: input.modelMaxBudget }),
      ...(values.budget_duration === NEVER_RESETS_BUDGET_DURATION && { budget_duration: null }),
    },
  };
};
