import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import useCan from "@/app/(dashboard)/hooks/useCan";
import { organizationKeys, useOrganizations } from "@/app/(dashboard)/hooks/organizations/useOrganizations";
import { useQueryClient } from "@tanstack/react-query";
import UserSearchModal from "@/components/common_components/user_search_modal";
import {
  Member,
  Organization,
  organizationInfoCall,
  teamInfoCall,
  teamMemberAddCall,
  teamMemberDeleteCall,
  teamMemberUpdateCall,
  teamUpdateCall,
} from "@/components/networking";
import { useGuardrails } from "@/app/(dashboard)/hooks/guardrails/useGuardrails";
import { formatNumberWithCommas } from "@/utils/dataUtils";
import type { ObjectPermission } from "@/components/object_permission_types";
import { isProxyAdminRole, isTeamAdminRole } from "@/utils/roles";
import { ArrowLeftIcon } from "@heroicons/react/outline";
import { StatusBadge, type StatusTone } from "@/components/shared/table_cells/status_badge";
import { BadgeLink } from "@/components/shared/BadgeLink";
import { Badge } from "@/components/ui/badge";
import { modelGroupHref } from "@/utils/entityLinks";
import { Card } from "@/components/ui/card";
import { Input as UIInput } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { SimpleTooltip, TooltipProvider } from "@/components/ui/tooltip";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { FieldGroup } from "@/components/ui/field";
import { FormField } from "@/components/shared/form/FormField";
import { useZodForm } from "@/lib/forms/useZodForm";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useVisitedTabs } from "@/hooks/useVisitedTabs";
import { toast } from "@/lib/toast";
import { CheckIcon, CopyIcon, Pencil, Save } from "lucide-react";
import React, { useEffect, useMemo, useState } from "react";
import { useFieldArray } from "react-hook-form";
import { z } from "zod/v4";
import { copyToClipboard as utilCopyToClipboard } from "../../utils/dataUtils";
import {
  computeTeamModelBadges,
  TeamAccessGroupModelGrant,
  TeamModelBadge,
  TeamModelBadgeKind,
} from "./teamModelAccess";

import {
  metadataObjectToPairs,
  metadataPairsSchema,
} from "../common_components/MetadataKeyValueFields";
import { useTeamMetadataSchema } from "@/app/(dashboard)/hooks/teams/useTeamMetadataSchema";
import DeleteResourceModal from "../common_components/DeleteResourceModal";
import { unfurlWildcardModelsInList } from "../key_team_helpers/fetch_available_models_team_key";

import {
  mcpServersForIdentifier,
  type EffectiveMcpServer,
} from "../mcp_server_management/effectiveMcpServers";
import type { MCPServer } from "../mcp_tools/types";
import { type AccessGroupResponse } from "@/app/(dashboard)/hooks/accessGroups/useAccessGroups";
import { ModelSelect } from "../ModelSelect/ModelSelect";
import { estimateChecks, estimateTooltips } from "../templates/estimatedOutputTokens";

import NumericalInput from "../shared/numerical_input";
import { RouterSettingsAccordionRef } from "../common_components/RouterSettingsAccordion";
import MemberModal from "./EditMembership";

import {
  getTeamInfoDefaultTab,
  getTeamInfoVisibleTabs,
  TEAM_INFO_TAB_KEYS,
  TEAM_INFO_TAB_LABELS,
} from "./tabVisibilityUtils";
import TeamMembersComponent from "./TeamMemberTab";
import { TeamVirtualKeysTable } from "./TeamVirtualKeysTable";
import { t } from "@/i18n";

const UI_MANAGED_METADATA_KEYS: ReadonlySet<string> = new Set([
  "logging",
  "secret_manager_settings",
  "soft_budget_alerting_emails",
  "model_tpm_limit",
  "model_rpm_limit",
  "default_estimated_output_tokens",
  "default_estimated_output_tokens_per_model",
  "allowed_passthrough_routes",
  "guardrails",
  "opted_out_global_guardrails",
  "disable_global_guardrails",
]);

const TEAM_MODEL_BADGE_TONES: Record<TeamModelBadgeKind, StatusTone> = {
  "all-proxy": "error",
  "no-default": "neutral",
  direct: "info",
  "access-group": "success",
};

const teamModelBadgeHref = (badge: TeamModelBadge): string | undefined =>
  badge.kind === "direct" || badge.kind === "access-group" ? modelGroupHref(badge.label) : undefined;

export type McpGrantResolution =
  | { readonly kind: "resolved"; readonly serverIds: ReadonlySet<string> }
  | { readonly kind: "unresolvable"; readonly reason: string };

export type TeamAccessGroupGrants = {
  readonly ids: readonly string[];
  readonly serverIds: readonly string[];
};

const sameIdSelection = (a: readonly string[], b: readonly string[]): boolean => {
  const aSet = new Set(a);
  const bSet = new Set(b);
  return aSet.size === bSet.size && [...aSet].every((id) => bSet.has(id));
};

export const standingToolPermissionServerIds = (
  loadedEffectiveServers: readonly EffectiveMcpServer[],
  loadedAccessGroupIds: readonly string[],
  accessGroups: readonly Pick<AccessGroupResponse, "access_group_id" | "access_mcp_server_ids">[],
  loadedTeamGroupServerIds: readonly string[],
): ReadonlySet<string> => {
  const loadedUnifiedServerIds = new Set([
    ...accessGroups
      .filter((group) => loadedAccessGroupIds.includes(group.access_group_id))
      .flatMap((group) => group.access_mcp_server_ids),
    ...loadedTeamGroupServerIds,
  ]);
  return new Set(
    loadedEffectiveServers
      .filter(({ source, server }) => source.kind === "toolPermission" && !loadedUnifiedServerIds.has(server.server_id))
      .map(({ server }) => server.server_id),
  );
};

export type McpGrantInput = {
  readonly effectiveServers: readonly EffectiveMcpServer[];
  readonly selectedAccessGroupIds: readonly string[];
  readonly accessGroups: readonly Pick<AccessGroupResponse, "access_group_id" | "access_mcp_server_ids">[];
  readonly standingServerIds: ReadonlySet<string>;
  readonly loadTeamGroups: () => Promise<TeamAccessGroupGrants>;
};

export const grantedMcpServerIds = async ({
  effectiveServers,
  selectedAccessGroupIds,
  accessGroups,
  standingServerIds,
  loadTeamGroups,
}: McpGrantInput): Promise<McpGrantResolution> => {
  const selectedGroups = accessGroups.filter((group) => selectedAccessGroupIds.includes(group.access_group_id));
  const direct = effectiveServers
    .filter(({ source }) => source.kind !== "toolPermission")
    .map(({ server }) => server.server_id);
  if (selectedAccessGroupIds.every((id) => selectedGroups.some((group) => group.access_group_id === id))) {
    return {
      kind: "resolved",
      serverIds: new Set([
        ...direct,
        ...selectedGroups.flatMap((group) => group.access_mcp_server_ids),
        ...standingServerIds,
      ]),
    };
  }
  const loadedTeamGroups = await loadTeamGroups().catch(() => null);
  if (loadedTeamGroups === null) {
    return { kind: "unresolvable", reason: "the team's access groups could not be reloaded" };
  }
  if (sameIdSelection(selectedAccessGroupIds, loadedTeamGroups.ids)) {
    return {
      kind: "resolved",
      serverIds: new Set([...direct, ...loadedTeamGroups.serverIds, ...standingServerIds]),
    };
  }
  return { kind: "unresolvable", reason: "the team's access groups could not be loaded" };
};

export const retainedMcpToolPermissions = (
  toolPermissions: Record<string, string[]>,
  grantedServerIds: ReadonlySet<string>,
  knownServers: readonly MCPServer[],
): Record<string, string[]> => {
  const entries = Object.entries(toolPermissions).flatMap(([key, tools]) => {
    const named = mcpServersForIdentifier(knownServers, key);
    const granted = named.filter((server) => grantedServerIds.has(server.server_id));
    if (named.length === 0 || granted.length === named.length) {
      return [[key, tools] as const];
    }
    if (granted.length === 0) {
      return [];
    }
    return granted.map(({ server_id }) => [server_id, [...(toolPermissions[server_id] ?? []), ...tools]] as const);
  });
  return entries.reduce<Record<string, string[]>>(
    (retained, [key, tools]) => ({
      ...retained,
      [key]: [...new Set([...(retained[key] ?? []), ...tools])],
    }),
    {},
  );
};

export const mcpUnresolvableSaveError = (reason: string): string =>
  `Cannot save MCP tool permissions because ${reason}. Retry once the page has finished loading`;

export interface TeamMembership {
  user_id: string;
  team_id: string;
  budget_id: string;
  spend: number;
  total_spend: number | null;
  litellm_budget_table: {
    budget_id: string;
    soft_budget: number | null;
    max_budget: number | null;
    max_parallel_requests: number | null;
    tpm_limit: number | null;
    rpm_limit: number | null;
    model_max_budget: Record<string, number> | null;
    budget_duration: string | null;
    budget_reset_at: string | null;
    allowed_models?: string[] | null;
  };
}

export interface TeamData {
  team_id: string;
  team_info: {
    team_alias: string;
    description?: string;
    team_id: string;
    organization_id: string | null;
    admins: string[];
    members: string[];
    members_with_roles: Member[];
    metadata: Record<string, any>;
    tpm_limit: number | null;
    rpm_limit: number | null;
    max_budget: number | null;
    soft_budget?: number | null;
    budget_duration: string | null;
    models: string[];
    blocked: boolean;
    spend: number;
    max_parallel_requests: number | null;
    budget_reset_at: string | null;
    model_id: string | null;
    litellm_model_table: {
      model_aliases: Record<string, string> | null;
    } | null;
    created_at: string;
    access_group_ids?: string[];
    default_team_member_models?: string[];
    access_group_models?: string[];
    access_group_mcp_server_ids?: string[];
    access_group_agent_ids?: string[];
    access_group_details?: TeamAccessGroupModelGrant[];
    router_settings?: Record<string, any>;
    guardrails?: string[];
    policies?: string[];
    object_permission?: ObjectPermission | null;
    team_member_budget_table: {
      max_budget: number;
      budget_duration: string | null;
      tpm_limit: number | null;
      rpm_limit: number | null;
    } | null;
  };
  keys: any[];
  team_memberships: TeamMembership[];
}

export interface TeamInfoProps {
  teamId: string;
  onUpdate: (data: any) => void;
  onClose: () => void;
  accessToken: string | null;
  is_team_admin: boolean;
  is_proxy_admin: boolean;
  is_org_admin?: boolean;
  userModels: string[];
  editTeam: boolean;
  premiumUser?: boolean;
}

const SUPPRESSED_BY_DESCRIPTION = "";

const numericInputSchema = z.union([z.string(), z.number()]).nullish();

const teamUpdateFieldsSchema = z.object({
  team_alias: z.string().min(1, t("Please input a team name")),
  team_description: z.string().optional(),
  blocked: z.boolean().optional(),
  models: z.array(z.string()).optional(),
  max_budget: numericInputSchema,
  soft_budget: numericInputSchema,
  soft_budget_alerting_emails: z.union([z.string(), z.array(z.string())]).optional(),
  default_team_member_models: z.array(z.string()).optional(),
  team_member_budget: numericInputSchema,
  team_member_budget_duration: z.string().nullish(),
  team_member_key_duration: z.string().optional(),
  team_member_tpm_limit: numericInputSchema,
  team_member_rpm_limit: numericInputSchema,
  budget_duration: z.string().nullish(),
  tpm_limit: numericInputSchema,
  rpm_limit: numericInputSchema,
  modelLimits: z
    .array(
      z.object({
        model: z
          .string()
          .nullable()
          .refine((model) => Boolean(model), t("Missing model")),
        tpm: z.number().nullish(),
        rpm: z.number().nullish(),
      }),
    )
    .superRefine((rows, ctx) => {
      rows.forEach((row, index) => {
        if (row.model && rows.filter((other) => other.model === row.model).length > 1) {
          ctx.addIssue({ code: "custom", message: t("Duplicate model"), path: [index, "model"] });
        }
        if (row.model && row.tpm == null && row.rpm == null) {
          ctx.addIssue({ code: "custom", message: t("Set at least one of TPM or RPM"), path: [index, "tpm"] });
        }
      });
    }),
  default_estimated_output_tokens: numericInputSchema.refine(
    estimateChecks.positive.isValid,
    estimateChecks.positive.message,
  ),
  default_estimated_output_tokens_per_model: z
    .string()
    .optional()
    .refine(estimateChecks.perModel.isValid, estimateChecks.perModel.message),
  guardrails: z.array(z.string()).optional(),
  disable_global_guardrails: z.boolean().optional(),
  policies: z.array(z.string()).optional(),
  access_group_ids: z.array(z.string()).optional(),
  vector_stores: z.array(z.string()).optional(),
  allowed_passthrough_routes: z.array(z.string()).optional(),
  mcp_servers_and_groups: z
    .object({
      servers: z.array(z.string()),
      accessGroups: z.array(z.string()),
      toolsets: z.array(z.string()).optional(),
    })
    .optional(),
  mcp_tool_permissions: z.record(z.string(), z.array(z.string())).optional(),
  agents_and_groups: z.object({ agents: z.array(z.string()), accessGroups: z.array(z.string()) }).optional(),
  object_permission_search_tools: z.array(z.string()).optional(),
  object_permission_skills: z.array(z.string()).optional(),
  organization_id: z.string().nullish(),
  logging_settings: z.array(z.unknown()).optional(),
  secret_manager_settings: z.string().optional(),
  metadata: metadataPairsSchema.optional(),
});

type TeamUpdateFormValues = z.infer<typeof teamUpdateFieldsSchema>;

type TeamInfoRecord = TeamData["team_info"] & { team_member_key_duration?: string };

const TEAM_MEMBER_SETTINGS_FIELDS = [
  "default_team_member_models",
  "team_member_budget",
  "team_member_budget_duration",
  "team_member_key_duration",
  "team_member_tpm_limit",
  "team_member_rpm_limit",
] as const;
const SEARCH_TOOL_SETTINGS_FIELDS = ["object_permission_search_tools"] as const;

const EMPTY_TEAM_UPDATE_VALUES: TeamUpdateFormValues = {
  team_alias: "",
  team_description: "",
  blocked: false,
  models: [],
  max_budget: undefined,
  soft_budget: undefined,
  soft_budget_alerting_emails: "",
  default_team_member_models: [],
  team_member_budget: undefined,
  team_member_budget_duration: undefined,
  team_member_key_duration: undefined,
  team_member_tpm_limit: undefined,
  team_member_rpm_limit: undefined,
  budget_duration: undefined,
  tpm_limit: undefined,
  rpm_limit: undefined,
  modelLimits: [],
  default_estimated_output_tokens: undefined,
  default_estimated_output_tokens_per_model: "",
  guardrails: [],
  disable_global_guardrails: false,
  policies: [],
  access_group_ids: [],
  vector_stores: [],
  allowed_passthrough_routes: [],
  mcp_servers_and_groups: { servers: [], accessGroups: [], toolsets: [] },
  mcp_tool_permissions: {},
  agents_and_groups: { agents: [], accessGroups: [] },
  object_permission_search_tools: [],
  object_permission_skills: [],
  organization_id: null,
  logging_settings: [],
  secret_manager_settings: "",
  metadata: [],
};

const computeEffectiveGuardrails = (info: TeamInfoRecord, globalGuardrailNames: ReadonlySet<string>): string[] => {
  const optedOutGlobals = new Set<string>(
    Array.isArray(info.metadata?.opted_out_global_guardrails) ? info.metadata.opted_out_global_guardrails : [],
  );
  const nonGlobalOptIns: string[] = (Array.isArray(info.metadata?.guardrails) ? info.metadata.guardrails : []).filter(
    (name: string) => !globalGuardrailNames.has(name),
  );
  return info.metadata?.disable_global_guardrails === true
    ? nonGlobalOptIns
    : [...Array.from(globalGuardrailNames).filter((name) => !optedOutGlobals.has(name)), ...nonGlobalOptIns];
};

const toTeamFormValues = (info: TeamInfoRecord, effectiveGuardrails: string[]): TeamUpdateFormValues => ({
  team_alias: info.team_alias,
  team_description: info.description ?? "",
  blocked: info.blocked === true,
  models: info.models,
  max_budget: info.max_budget,
  soft_budget: info.soft_budget,
  soft_budget_alerting_emails: Array.isArray(info.metadata?.soft_budget_alerting_emails)
    ? info.metadata.soft_budget_alerting_emails.join(", ")
    : "",
  default_team_member_models: info.default_team_member_models || [],
  team_member_budget: info.team_member_budget_table?.max_budget,
  team_member_budget_duration: info.team_member_budget_table?.budget_duration,
  team_member_key_duration: info.metadata?.team_member_key_duration,
  team_member_tpm_limit: info.team_member_budget_table?.tpm_limit,
  team_member_rpm_limit: info.team_member_budget_table?.rpm_limit,
  budget_duration: info.budget_duration,
  tpm_limit: info.tpm_limit,
  rpm_limit: info.rpm_limit,
  modelLimits: Array.from(
    new Set([
      ...Object.keys(info.metadata?.model_tpm_limit ?? {}),
      ...Object.keys(info.metadata?.model_rpm_limit ?? {}),
    ]),
  ).map((model) => ({
    model,
    tpm: info.metadata?.model_tpm_limit?.[model],
    rpm: info.metadata?.model_rpm_limit?.[model],
  })),
  default_estimated_output_tokens: info.metadata?.default_estimated_output_tokens,
  default_estimated_output_tokens_per_model: info.metadata?.default_estimated_output_tokens_per_model
    ? JSON.stringify(info.metadata.default_estimated_output_tokens_per_model)
    : "",
  guardrails: effectiveGuardrails,
  disable_global_guardrails: info.metadata?.disable_global_guardrails || false,
  policies: info.policies || [],
  access_group_ids: info.access_group_ids || [],
  vector_stores: info.object_permission?.vector_stores || [],
  allowed_passthrough_routes: info.metadata?.allowed_passthrough_routes || [],
  mcp_servers_and_groups: {
    servers: info.object_permission?.mcp_servers || [],
    accessGroups: info.object_permission?.mcp_access_groups || [],
    toolsets: info.object_permission?.mcp_toolsets || [],
  },
  mcp_tool_permissions: info.object_permission?.mcp_tool_permissions || {},
  agents_and_groups: {
    agents: info.object_permission?.agents || [],
    accessGroups: info.object_permission?.agent_access_groups || [],
  },
  object_permission_search_tools: info.object_permission?.search_tools || [],
  object_permission_skills: info.object_permission?.skills || [],
  organization_id: info.organization_id,
  logging_settings: info.metadata?.logging || [],
  secret_manager_settings: info.metadata?.secret_manager_settings
    ? JSON.stringify(info.metadata.secret_manager_settings, null, 2)
    : "",
  metadata: metadataObjectToPairs(info.metadata, UI_MANAGED_METADATA_KEYS),
});

const isParsableJson = (value: string | undefined): boolean => {
  if (!value) {
    return true;
  }
  try {
    JSON.parse(value);
    return true;
  } catch {
    return false;
  }
};

const TeamInfoView: React.FC<TeamInfoProps> = ({
  teamId,
  onClose,
  accessToken,
  is_team_admin,
  is_proxy_admin,
  is_org_admin = false,
  userModels,
  editTeam,
  premiumUser = false,
  onUpdate,
}) => {
  const teamUpdateSchema = useMemo(
    () =>
      teamUpdateFieldsSchema.superRefine((values, ctx) => {
        if (!isParsableJson(values.secret_manager_settings)) {
          ctx.addIssue({ code: "custom", message: SUPPRESSED_BY_DESCRIPTION, path: ["secret_manager_settings"] });
        }
      }),
    [],
  );
  const [teamData, setTeamData] = useState<TeamData | null>(null);
  const [loading, setLoading] = useState(true);
  const [isAddMemberModalVisible, setIsAddMemberModalVisible] = useState(false);
  const form = useZodForm(teamUpdateSchema, { defaultValues: EMPTY_TEAM_UPDATE_VALUES });
  const {
    fields: modelLimitRows,
    append: appendModelLimit,
    remove: removeModelLimit,
  } = useFieldArray({ control: form.control, name: "modelLimits" });
  const [teamMemberSettingsOpen, setTeamMemberSettingsOpen] = useState(false);
  const [searchToolSettingsOpen, setSearchToolSettingsOpen] = useState(false);
  const [isEditMemberModalVisible, setIsEditMemberModalVisible] = useState(false);
  const [selectedEditMember, setSelectedEditMember] = useState<Member | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [copiedStates, setCopiedStates] = useState<Record<string, boolean>>({});
  const { data: guardrailsData, isLoading: isGuardrailsLoading } = useGuardrails();
  const globalGuardrailNames = guardrailsData?.globalGuardrailNames ?? new Set<string>();
  const canViewPolicies = useCan("viewPolicies");
  const [policiesList, setPoliciesList] = useState<string[]>([]);
  const [policyGuardrails, setPolicyGuardrails] = useState<Record<string, string[]>>({});
  const [loadingPolicies, setLoadingPolicies] = useState(false);
  const [memberToDelete, setMemberToDelete] = useState<Member | null>(null);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isTeamSaving, setIsTeamSaving] = useState(false);
  const [teamModelAliases, setTeamModelAliases] = useState<Record<string, string>>({});
  const routerSettingsRef = React.useRef<RouterSettingsAccordionRef>(null);
  const [organization, setOrganization] = useState<Organization | null>(null);
  const { userRole, userId } = useAuthorized();

  const canEditTeamEstimates = isProxyAdminRole(userRole);
  const teamEstimateTooltip = estimateTooltips(canEditTeamEstimates, "team");
  const { data: userOrganizations = [] } = useOrganizations();
  const { data: teamMetadataSchemaFields = [], isLoading: isTeamMetadataSchemaLoading } = useTeamMetadataSchema();
  const queryClient = useQueryClient();

  // Check if user is org admin for this team's organization
  const isOrgAdminForTeam = useMemo(() => {
    const teamOrgId = teamData?.team_info?.organization_id;
    if (!teamOrgId || !userId) return false;
    const org = userOrganizations.find((o) => o.organization_id === teamOrgId);
    return org?.members?.some((m: any) => m.user_id === userId && m.user_role === "org_admin") ?? false;
  }, [teamData, userOrganizations, userId]);

  // Models currently selected in the team edit form, used to scope the per-model
  // rate limit dropdown to models this team actually has access to.
  const watchedModels = form.watch("models");
  const killSwitchOn = form.watch("disable_global_guardrails");
  const availableRateLimitModels = useMemo(() => {
    const selected = watchedModels ?? teamData?.team_info?.models ?? [];
    if (selected.includes("all-proxy-models") || selected.includes("all-team-models")) {
      return userModels;
    }
    return unfurlWildcardModelsInList(selected, userModels);
  }, [watchedModels, teamData, userModels]);

  const isTeamAdminFromTeamData = useMemo(
    () =>
      teamData?.team_info?.members_with_roles?.some(
        (member) => member.user_id != null && member.user_id === userId && isTeamAdminRole(member.role),
      ) ?? false,
    [teamData, userId],
  );

  const canEditTeam = is_team_admin || is_proxy_admin || is_org_admin || isOrgAdminForTeam || isTeamAdminFromTeamData;
  const visibleTabs = useMemo(() => getTeamInfoVisibleTabs(canEditTeam), [canEditTeam]);
  const defaultTabKey = useMemo(() => getTeamInfoDefaultTab(editTeam, canEditTeam), [editTeam, canEditTeam]);
  const { onTabChange, hasVisited } = useVisitedTabs(defaultTabKey);

  const teamFormValues = (): TeamUpdateFormValues => {
    const info = teamData?.team_info;
    return info
      ? toTeamFormValues(info, computeEffectiveGuardrails(info, globalGuardrailNames))
      : EMPTY_TEAM_UPDATE_VALUES;
  };

  const startEditing = () => {
    form.reset(teamFormValues());
    setTeamMemberSettingsOpen(false);
    setSearchToolSettingsOpen(false);
    setIsEditing(true);
  };

  const applyKillSwitchToGuardrails = (checked: boolean) => {
    const current = form.getValues("guardrails") ?? [];
    const nonGlobals = current.filter((name) => !globalGuardrailNames.has(name));
    form.setValue("guardrails", checked ? nonGlobals : [...Array.from(globalGuardrailNames), ...nonGlobals]);
  };

  const mountedUpdateValues = (values: TeamUpdateFormValues): Record<string, unknown> => {
    const unmounted = new Set<string>([
      ...(teamMemberSettingsOpen ? [] : TEAM_MEMBER_SETTINGS_FIELDS),
      ...(canViewPolicies ? [] : ["policies"]),
      ...(searchToolSettingsOpen ? [] : SEARCH_TOOL_SETTINGS_FIELDS),
    ]);
    return Object.fromEntries(Object.entries(values).filter(([key]) => !unmounted.has(key)));
  };

  const onTeamUpdateSubmit = (values: TeamUpdateFormValues) => handleTeamUpdate(mountedUpdateValues(values));

  const fetchTeamInfo = async () => {
    try {
      setLoading(true);
      if (!accessToken) return;
      const response = await teamInfoCall(accessToken, teamId);
      setTeamData(response);
    } catch (error) {
      toast.fromError(t("Failed to load team information"));
      console.error("Error fetching team info:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTeamInfo();
  }, [teamId, accessToken]);

  // Fetch organization data when team has organization_id
  useEffect(() => {
    const fetchOrganization = async () => {
      if (!accessToken || !teamData?.team_info?.organization_id) {
        setOrganization(null);
        return;
      }

      try {
        const orgData = await organizationInfoCall(accessToken, teamData.team_info.organization_id);
        setOrganization(orgData);
      } catch (error) {
        console.error("Error fetching organization info:", error);
        setOrganization(null);
      }
    };

    fetchOrganization();
  }, [accessToken, teamData?.team_info?.organization_id]);



  const handleMemberCreate = async (values: any) => {
    try {
      if (accessToken == null) return;

      const member: Member = {
        user_email: values.user_email,
        user_id: values.user_id,
        role: values.role,
      };

      await teamMemberAddCall(accessToken, teamId, member);

      toast.success(t("Team member added successfully"));
      setIsAddMemberModalVisible(false);
      form.reset(teamFormValues());

      // Fetch updated team info
      const updatedTeamData = await teamInfoCall(accessToken, teamId);
      setTeamData(updatedTeamData);

      // Notify parent component of the update
      onUpdate(updatedTeamData);
    } catch (error: any) {
      let errMsg = t("Failed to add team member");

      if (error?.raw?.detail?.error?.includes("Assigning team admins is a premium feature")) {
        errMsg = t("Assigning admins is an enterprise-only feature. Please upgrade your LiteLLM plan to enable this.");
      } else if (error?.message) {
        errMsg = error.message;
      }

      toast.fromError(errMsg);
      console.error("Error adding team member:", error);
    }
  };

  const handleMemberUpdate = async (values: any) => {
    try {
      if (accessToken == null) {
        return;
      }

      const member: Member = {
        user_email: values.user_email,
        user_id: values.user_id,
        role: values.role,
        max_budget_in_team: values.max_budget_in_team,
        tpm_limit: values.tpm_limit,
        rpm_limit: values.rpm_limit,
        budget_duration: values.budget_duration,
        allowed_models: values.allowed_models,
      };
      toast.dismiss(); // Remove all existing toasts

      await teamMemberUpdateCall(accessToken, teamId, member);

      toast.success(t("Team member updated successfully"));
      setIsEditMemberModalVisible(false);

      // Fetch updated team info
      const updatedTeamData = await teamInfoCall(accessToken, teamId);
      setTeamData(updatedTeamData);

      // Notify parent component of the update
      onUpdate(updatedTeamData);
    } catch (error: any) {
      let errMsg = t("Failed to update team member");
      if (error?.raw?.detail?.includes("Assigning team admins is a premium feature")) {
        errMsg = t("Assigning admins is an enterprise-only feature. Please upgrade your LiteLLM plan to enable this.");
      } else if (error?.message) {
        errMsg = error.message;
      }
      setIsEditMemberModalVisible(false);

      toast.dismiss(); // Remove all existing toasts

      toast.fromError(errMsg);
      console.error("Error updating team member:", error);
    }
  };

  const handleMemberDelete = (member: Member) => {
    setMemberToDelete(member);
    setIsDeleteModalOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!memberToDelete || !accessToken) return;

    setIsDeleting(true);
    try {
      await teamMemberDeleteCall(accessToken, teamId, memberToDelete);

      toast.success(t("Team member removed successfully"));

      // Fetch updated team info
      const updatedTeamData = await teamInfoCall(accessToken, teamId);
      setTeamData(updatedTeamData);

      // Notify parent component of the update
      onUpdate(updatedTeamData);
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      toast.fromError(message ? t(message) : t("Failed to remove team member"));
      console.error("Error removing team member:", error);
    } finally {
      setIsDeleting(false);
      setIsDeleteModalOpen(false);
      setMemberToDelete(null);
    }
  };

  const handleDeleteCancel = () => {
    setIsDeleteModalOpen(false);
    setMemberToDelete(null);
  };

  const handleTeamUpdate = async (values: any) => {
    try {
      if (!accessToken) return;
      setIsTeamSaving(true);

      const sanitizeNumeric = (v: unknown) => {
        if (v === null || v === undefined) return null;
        if (typeof v === "string" && v.trim() === "") return null;
        if (typeof v === "number" && Number.isNaN(v)) return null;
        return v;
      };

      // The gateway only stores the name, the description, and — for a platform
      // administrator — the model list, the budget ceiling and blocked. Sending
      // models or a budget from a team admin makes the whole save fail.
      const updateData: Record<string, unknown> = {
        team_id: teamId,
        team_alias: values.team_alias,
        team_description: values.team_description ?? "",
      };
      if (is_proxy_admin) {
        updateData.models = Array.isArray(values.models) ? values.models : [];
        updateData.max_budget = sanitizeNumeric(values.max_budget);
        updateData.blocked = values.blocked === true;
      }

      await teamUpdateCall(accessToken, updateData);
      queryClient.invalidateQueries({ queryKey: organizationKeys.all });

      toast.success(t("Team settings updated successfully"));
      setIsEditing(false);
      fetchTeamInfo();
      return;
    } catch (error) {
      console.error("Error updating team:", error);
      toast.fromError(error instanceof Error ? error.message : t("Failed to update team settings: "));
    } finally {
      setIsTeamSaving(false);
    }
  };


  if (loading) {
    return <div className="p-4">{t("Loading...")}</div>;
  }

  if (!teamData?.team_info) {
    return <div className="p-4">{t("Team not found")}</div>;
  }

  const { team_info: info } = teamData;

  const copyToClipboard = async (text: string, key: string) => {
    const success = await utilCopyToClipboard(text);
    if (success) {
      setCopiedStates((prev) => ({ ...prev, [key]: true }));
      setTimeout(() => {
        setCopiedStates((prev) => ({ ...prev, [key]: false }));
      }, 2000);
    }
  };

  const tabItems = [
    {
      key: TEAM_INFO_TAB_KEYS.OVERVIEW,
      label: t(TEAM_INFO_TAB_LABELS[TEAM_INFO_TAB_KEYS.OVERVIEW]),
      children: (
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
          <Card className="block p-6">
            <p>{t("Budget Status")}</p>
            <div className="mt-2">
              <h3 className="text-lg font-medium">${formatNumberWithCommas(info.spend, 2)}</h3>
              <p className="text-muted-foreground">
                {t("Budget cap: {value0}", {
                  value0: info.max_budget === null ? t("Unlimited") : `$${formatNumberWithCommas(info.max_budget, 2)}`,
                })}
              </p>
            </div>
          </Card>

          <Card className="block p-6">
            <p>{t("Models")}</p>
            <div className="mt-2 flex flex-wrap gap-2">
              {computeTeamModelBadges(info.models, info.access_group_models || [], info.access_group_details).map(
                (badge, index) => (
                  <SimpleTooltip key={`${badge.kind}-${badge.label}-${index}`} content={badge.tooltip}>
                    <span>
                      <StatusBadge
                        tone={TEAM_MODEL_BADGE_TONES[badge.kind]}
                        label={badge.label}
                        href={teamModelBadgeHref(badge)}
                      />
                    </span>
                  </SimpleTooltip>
                ),
              )}
            </div>
          </Card>

          <Card className="block p-6">
            <p>{t("Members")}</p>
            <p className="mt-2 text-lg font-medium">{info.members_with_roles?.length ?? 0}</p>
          </Card>

          <Card className="block p-6">
            <p className="font-semibold text-foreground">{t("Virtual Keys")}</p>
            <p className="mt-2">{t("Total: {value0}", { value0: teamData.keys.length })}</p>
          </Card>

          <Card className="block p-6 sm:col-span-2">
            <p>{t("Description")}</p>
            <p className="mt-2 whitespace-pre-wrap text-muted-foreground">{info.description || t("None")}</p>
          </Card>
        </div>
      ),
    },
    {
      key: TEAM_INFO_TAB_KEYS.VIRTUAL_KEYS,
      label: t(TEAM_INFO_TAB_LABELS[TEAM_INFO_TAB_KEYS.VIRTUAL_KEYS]),
      children: <TeamVirtualKeysTable teamId={teamId} teamAlias={info.team_alias} organization={organization} />,
    },
    {
      key: TEAM_INFO_TAB_KEYS.MEMBERS,
      label: t(TEAM_INFO_TAB_LABELS[TEAM_INFO_TAB_KEYS.MEMBERS]),
      children: (
        <TeamMembersComponent
          teamData={teamData}
          canEditTeam={canEditTeam}
          handleMemberDelete={handleMemberDelete}
          setSelectedEditMember={setSelectedEditMember}
          setIsEditMemberModalVisible={setIsEditMemberModalVisible}
          setIsAddMemberModalVisible={setIsAddMemberModalVisible}
        />
      ),
    },
    {
      key: TEAM_INFO_TAB_KEYS.SETTINGS,
      label: t(TEAM_INFO_TAB_LABELS[TEAM_INFO_TAB_KEYS.SETTINGS]),
      children: (
        <Card className="block p-6 overflow-y-auto max-h-[65vh]">
          <div className="flex justify-between items-center mb-4">
            <h3 className="text-lg font-medium">{t("Team Settings")}</h3>
            {canEditTeam && !isEditing && (
              <Button
                variant="outline"
                onClick={() => {
                  setTeamModelAliases(info.litellm_model_table?.model_aliases ?? {});
                  startEditing();
                }}
              >
                <Pencil />
                {t("Edit Settings")}
              </Button>
            )}
          </div>

          {isEditing ? (
            <TooltipProvider>
              <form onSubmit={(event) => void form.handleSubmit(onTeamUpdateSubmit)(event)}>
                <FieldGroup>
                  <FormField control={form.control} name="team_alias" label={t("Team Name")}>
                    {({ ref, value, ...field }) => <UIInput {...field} ref={ref} value={value ?? ""} />}
                  </FormField>

                  <FormField control={form.control} name="team_description" label={t("Description")}>
                    {({ ref, value, ...field }) => <Textarea {...field} ref={ref} value={value ?? ""} rows={3} />}
                  </FormField>

                  <FormField
                    control={form.control}
                    name="models"
                    label={t("Models")}
                    description={t(
                      "Leave the model list empty to allow every published model. A list limits the team to those models.",
                    )}
                  >
                    {({ id, value, onChange }) => (
                      <ModelSelect
                        id={id}
                        value={value ?? []}
                        onChange={onChange}
                        teamID={teamId}
                        organizationID={teamData?.team_info?.organization_id || undefined}
                        options={{
                          includeSpecialOptions: false,
                          includeUserModels: !teamData?.team_info?.organization_id,
                        }}
                        context="team"
                        dataTestId="models-select"
                        disabled={!is_proxy_admin}
                      />
                    )}
                  </FormField>

                  <p className="text-sm text-muted-foreground">
                    {t("Only a platform administrator can change models and the budget ceiling.")}
                  </p>

                  <FormField control={form.control} name="max_budget" label={t("Max Budget (USD)")}>
                    {({ ref, value, ...field }) => (
                      <NumericalInput
                        {...field}
                        ref={ref}
                        value={value ?? ""}
                        step={0.01}
                        precision={2}
                        disabled={!is_proxy_admin}
                      />
                    )}
                  </FormField>

                  {is_proxy_admin ? (
                    <FormField control={form.control} name="blocked" label={t("Blocked")}>
                      {({ id, value, onChange }) => (
                        <Switch id={id} checked={value === true} onCheckedChange={onChange} />
                      )}
                    </FormField>
                  ) : null}
                </FieldGroup>
                <div className="mt-6 flex justify-end gap-2">
                  <Button type="button" variant="outline" onClick={() => setIsEditing(false)}>
                    {t("Cancel")}
                  </Button>
                  <Button type="submit" disabled={isTeamSaving}>
                    {isTeamSaving ? <UiLoadingSpinner className="size-4" /> : <Save className="size-4" />}
                    {t("Save Changes")}
                  </Button>
                </div>
              </form>
            </TooltipProvider>
          ) : (
            <div className="space-y-4">
              <div>
                <p className="font-medium">{t("Team Name")}</p>
                <div>{info.team_alias}</div>
              </div>
              <div>
                <p className="font-medium">{t("Description")}</p>
                <div className="whitespace-pre-wrap text-muted-foreground">{info.description || t("None")}</div>
              </div>
              <div>
                <p className="font-medium">{t("Team ID")}</p>
                <div className="font-mono">{info.team_id}</div>
              </div>
              <div>
                <p className="font-medium">{t("Created At")}</p>
                <div>{info.created_at ? new Date(info.created_at).toLocaleString() : t("None")}</div>
              </div>
              <div>
                <p className="font-medium">{t("Models")}</p>
                {info.models.length === 0 ? (
                  <div className="text-muted-foreground">
                    {t("Leave the model list empty to allow every published model. A list limits the team to those models.")}
                  </div>
                ) : (
                  <div className="mt-1 flex flex-wrap gap-2">
                    {info.models.map((model, index) => (
                      <BadgeLink key={index} href={modelGroupHref(model)}>
                        {model}
                      </BadgeLink>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <p className="font-medium">{t("Team Budget")}</p>
                <div>
                  {t("Max Budget: {value0}", {
                    value0:
                      info.max_budget !== null ? `$${formatNumberWithCommas(info.max_budget, 4)}` : t("No Limit"),
                  })}
                </div>
              </div>
              <div>
                <p className="font-medium">{t("Organization ID")}</p>
                <div>{info.organization_id || t("None")}</div>
              </div>
              <div>
                <p className="font-medium">{t("Status")}</p>
                <Badge variant={info.blocked ? "destructive" : "secondary"}>
                  {info.blocked ? t("Blocked") : t("Active")}
                </Badge>
              </div>
            </div>
          )}
        </Card>
      ),
    },
  ].filter((tab) => visibleTabs.includes(tab.key));

  return (
    <div className="p-4">
      <div className="flex justify-between items-center mb-6">
        <div>
          <Button variant="ghost" onClick={onClose} className="mb-4">
            <ArrowLeftIcon className="h-4 w-4" />
            {t("Back to Teams")}
          </Button>
          <h1 className="text-2xl font-semibold">{info.team_alias}</h1>
          <div className="flex items-center">
            <p className="text-sm text-muted-foreground font-mono">{info.team_id}</p>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => copyToClipboard(info.team_id, "team-id")}
              className={`left-2 z-raised transition-all duration-200 ${
                copiedStates["team-id"]
                  ? "text-success bg-success/10 border-success/20"
                  : "text-muted-foreground hover:text-foreground hover:bg-accent"
              }`}
            >
              {copiedStates["team-id"] ? <CheckIcon size={12} /> : <CopyIcon size={12} />}
            </Button>
          </div>
        </div>
      </div>

      <Tabs defaultValue={defaultTabKey} className="mb-4" onValueChange={onTabChange}>
        <TabsList variant="line" className="mb-4 h-auto w-full justify-start rounded-none border-b p-0">
          {tabItems.map(({ key, label }) => (
            <TabsTrigger key={key} value={key} className="flex-none rounded-none px-4 py-2">
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
        {tabItems.map(({ key, children }) => (
          <TabsContent key={key} value={key} keepMounted={hasVisited(key)}>
            {children}
          </TabsContent>
        ))}
      </Tabs>

      <MemberModal
        visible={isEditMemberModalVisible}
        onCancel={() => setIsEditMemberModalVisible(false)}
        onSubmit={handleMemberUpdate}
        initialData={selectedEditMember}
        mode="edit"
        config={{
          title: t("Edit Member"),
          showEmail: true,
          showUserId: true,
          roleOptions: [
            { label: t("Team admin"), value: "admin" },
            { label: t("Team member"), value: "user" },
          ],
        }}
      />

      <UserSearchModal
        isVisible={isAddMemberModalVisible}
        onCancel={() => setIsAddMemberModalVisible(false)}
        onSubmit={handleMemberCreate}
        accessToken={accessToken}
        teamId={teamId}
      />

      {/* Delete Member Confirmation Modal */}
      <DeleteResourceModal
        isOpen={isDeleteModalOpen}
        title={t("Delete Team Member")}
        alertMessage={t("Removing this member revokes the personal keys they hold for this team. Service keys stay.")}
        message={t("Are you sure you want to remove this member from the team? This action cannot be undone.")}
        resourceInformationTitle={t("Team Member Information")}
        resourceInformation={[
          { label: t("User ID"), value: memberToDelete?.user_id, code: true },
          { label: t("Email"), value: memberToDelete?.user_email },
          { label: t("Role"), value: memberToDelete?.role },
        ]}
        onCancel={handleDeleteCancel}
        onOk={handleDeleteConfirm}
        confirmLoading={isDeleting}
      />
    </div>
  );
};

export default TeamInfoView;
