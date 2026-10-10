import { isValidRateAllocation } from "./create_user_payload";
import { QuotaGuide } from "./shared/QuotaGuide";
import { useOrganizations } from "@/app/(dashboard)/hooks/organizations/useOrganizations";
import useCan from "@/app/(dashboard)/hooks/useCan";
import AvailableTeamsPanel from "@/components/team/AvailableTeamsPanel";
import TeamInfoView from "@/components/team/TeamInfo";
import { isTeamAdminRole } from "@/utils/roles";
import { Input as UIInput } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { TooltipProvider } from "@/components/ui/tooltip";
import { FieldGroup } from "@/components/ui/field";
import { FormField } from "@/components/shared/form/FormField";
import { SearchSelect } from "@/components/shared/SearchSelect";
import { t } from "@/i18n";
import { useZodForm } from "@/lib/forms/useZodForm";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Plus, Users } from "lucide-react";
import React, { useEffect, useMemo, useState } from "react";
import { z } from "zod/v4";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { PageHeader } from "@/components/shared/PageHeader";
import { Button as UIButton } from "@/components/ui/button";
import { teamsTableKeys } from "@/app/(dashboard)/hooks/teams/useTeams";
import { parseAsString, useQueryState } from "nuqs";
import { TeamsTable } from "./TeamsPage/TeamsTable";
import {
  metadataPairsSchema,
  metadataPairsToObject,
} from "./common_components/MetadataKeyValueFields";
import { useTeamMetadataSchema } from "@/app/(dashboard)/hooks/teams/useTeamMetadataSchema";

import { fetchAvailableModelsForTeamOrKey } from "./key_team_helpers/fetch_available_models_team_key";
import type { Team } from "./key_team_helpers/key_list";
import { toast } from "@/lib/toast";
import { extractProxyErrorMessage } from "@/lib/http/client";
import {
  getBudgetDurationLabel,
  NEVER_RESETS_BUDGET_DURATION,
} from "./common_components/budget_duration_dropdown";
import { Organization, getDefaultTeamSettings, getGuardrailsList, teamDeleteCall } from "./networking";
import NumericalInput from "./shared/numerical_input";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

interface TeamProps {
  accessToken: string | null;
  userID: string | null;
  userRole: string | null;
  premiumUser?: boolean;
}

import DeleteResourceModal from "./common_components/DeleteResourceModal";
import { teamCreateCall } from "./networking";
import RouteTemplateSelect from "./route_templates/RouteTemplateSelect";
import { ModelSelect } from "./ModelSelect/ModelSelect";

const SUPPRESSED_BY_DESCRIPTION = "";

const numericInputSchema = z.union([z.string(), z.number()]).optional();

const rateInputSchema = numericInputSchema.refine(isValidRateAllocation, { error: () => t("Must be a non-negative whole number") });

const teamCreateFieldsSchema = z.object({
  team_alias: z.string().min(1, t("Please input a team name")),
  team_description: z.string().optional(),
  organization_id: z.string().nullish(),
  models: z.array(z.string()).optional(),
  max_budget: numericInputSchema,
  budget_duration: z.string().nullish(),
  tpm_limit: rateInputSchema,
  rpm_limit: rateInputSchema,
  metadata: metadataPairsSchema.optional(),
  team_id: z.string().optional(),
  team_member_budget: z.number().optional(),
  team_member_key_duration: z.string().optional(),
  team_member_rpm_limit: numericInputSchema,
  team_member_tpm_limit: numericInputSchema,
  secret_manager_settings: z.string().optional(),
  guardrails: z.array(z.string()).optional(),
  disable_global_guardrails: z.boolean().optional(),
  policies: z.array(z.string()).optional(),
  access_group_ids: z.array(z.string()).optional(),
  allowed_vector_store_ids: z.array(z.string()).optional(),
  allowed_passthrough_routes: z.array(z.string()).optional(),
  allowed_mcp_servers_and_groups: z
    .object({
      servers: z.array(z.string()),
      accessGroups: z.array(z.string()),
      toolsets: z.array(z.string()).optional(),
    })
    .optional(),
  mcp_tool_permissions: z.record(z.string(), z.array(z.string())).optional(),
  allowed_agents_and_groups: z.object({ agents: z.array(z.string()), accessGroups: z.array(z.string()) }).optional(),
  object_permission_search_tools: z.array(z.string()).optional(),
  object_permission_skills: z.array(z.string()).optional(),
  route_template_id: z.string().optional(),
});

type TeamCreateFormValues = z.infer<typeof teamCreateFieldsSchema>;

const EMPTY_TEAM_CREATE_VALUES: TeamCreateFormValues = {
  team_alias: "",
  team_description: "",
  organization_id: null,
  models: [],
  max_budget: undefined,
  budget_duration: undefined,
  tpm_limit: undefined,
  rpm_limit: undefined,
  metadata: [],
  team_id: undefined,
  team_member_budget: undefined,
  team_member_key_duration: undefined,
  team_member_rpm_limit: undefined,
  team_member_tpm_limit: undefined,
  secret_manager_settings: undefined,
  guardrails: undefined,
  disable_global_guardrails: undefined,
  policies: undefined,
  access_group_ids: undefined,
  allowed_vector_store_ids: undefined,
  allowed_passthrough_routes: undefined,
  allowed_mcp_servers_and_groups: undefined,
  mcp_tool_permissions: {},
  allowed_agents_and_groups: undefined,
  object_permission_search_tools: undefined,
  object_permission_skills: undefined,
  route_template_id: "",
};

const ADDITIONAL_SETTINGS_FIELDS = [
  "team_id",
  "team_member_budget",
  "team_member_key_duration",
  "team_member_rpm_limit",
  "team_member_tpm_limit",
  "secret_manager_settings",
  "guardrails",
  "disable_global_guardrails",
  "policies",
  "access_group_ids",
  "allowed_vector_store_ids",
  "allowed_passthrough_routes",
] as const;
const MCP_SETTINGS_FIELDS = ["allowed_mcp_servers_and_groups", "mcp_tool_permissions"] as const;
const AGENT_SETTINGS_FIELDS = ["allowed_agents_and_groups"] as const;
const SEARCH_TOOL_SETTINGS_FIELDS = ["object_permission_search_tools"] as const;
const SKILL_SETTINGS_FIELDS = ["object_permission_skills"] as const;

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

const canCreateOrManageTeams = (
  userRole: string | null,
  userID: string | null,
  organizations: Organization[] | null,
): boolean => {
  // Admin role always has permission
  if (userRole === "Admin") {
    return true;
  }

  // Check if user is an org_admin in any organization
  if (organizations && userID) {
    return organizations.some((org) =>
      org.members?.some((member) => member.user_id === userID && member.user_role === "org_admin"),
    );
  }

  return false;
};

const getAdminOrganizations = (
  userRole: string | null,
  userID: string | null,
  organizations: Organization[] | null,
): Organization[] => {
  // Global Admin can see all organizations
  if (userRole === "Admin") {
    return organizations || [];
  }

  // Org Admin can only see organizations they're an admin for
  if (organizations && userID) {
    return organizations.filter((org) =>
      org.members?.some((member) => member.user_id === userID && member.user_role === "org_admin"),
    );
  }

  return [];
};

// @deprecated
const Teams: React.FC<TeamProps> = ({ accessToken, userID, userRole, premiumUser = false }) => {
  const { data: organizationsData } = useOrganizations();
  const organizations = organizationsData ?? null;
  const { data: teamMetadataSchemaFields = [], isLoading: isTeamMetadataSchemaLoading } = useTeamMetadataSchema();
  const queryClient = useQueryClient();
  const refreshTeams = () => queryClient.invalidateQueries({ queryKey: teamsTableKeys.all });
  const [currentOrg] = useState<Organization | null>(null);

  const isOrgAdmin = userRole !== "Admin";
  const [additionalSettingsOpen, setAdditionalSettingsOpen] = useState(false);
  const [mcpSettingsOpen, setMcpSettingsOpen] = useState(false);
  const [agentSettingsOpen, setAgentSettingsOpen] = useState(false);
  const [searchToolSettingsOpen, setSearchToolSettingsOpen] = useState(false);
  const [skillSettingsOpen, setSkillSettingsOpen] = useState(false);

  const adminOrgs = useMemo(
    () => getAdminOrganizations(userRole, userID, organizations),
    [userRole, userID, organizations],
  );

  const teamCreateSchema = useMemo(
    () =>
      teamCreateFieldsSchema.superRefine((values, ctx) => {
        if (isOrgAdmin && !values.organization_id) {
          ctx.addIssue({ code: "custom", message: SUPPRESSED_BY_DESCRIPTION, path: ["organization_id"] });
        }
        const organizationIsStillPickable =
          values.organization_id == null ||
          organizations == null ||
          adminOrgs.some((org) => org.organization_id === values.organization_id);
        if (!organizationIsStillPickable) {
          ctx.addIssue({
            code: "custom",
            message: t("You can no longer create teams in this organization"),
            path: ["organization_id"],
          });
        }
        if (additionalSettingsOpen && !isParsableJson(values.secret_manager_settings)) {
          ctx.addIssue({ code: "custom", message: SUPPRESSED_BY_DESCRIPTION, path: ["secret_manager_settings"] });
        }
      }),
    [isOrgAdmin, additionalSettingsOpen, adminOrgs, organizations],
  );

  const form = useZodForm(teamCreateSchema, { defaultValues: EMPTY_TEAM_CREATE_VALUES });
  const watchedOrganizationId = form.watch("organization_id");
  const watchedMcpSelection = form.watch("allowed_mcp_servers_and_groups");
  const watchedToolPermissions = form.watch("mcp_tool_permissions");

  const [selectedTeam, setSelectedTeam] = useState<Team | null>(null);
  const [selectedTeamId, setSelectedTeamId] = useQueryState("team", parseAsString.withOptions({ history: "push" }));
  const [editTeam, setEditTeam] = useState<boolean>(false);

  const [isTeamModalVisible, setIsTeamModalVisible] = useState(false);
  const [userModels, setUserModels] = useState<string[]>([]);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [teamToDelete, setTeamToDelete] = useState<Team | null>(null);
  const [isTeamDeleting, setIsTeamDeleting] = useState(false);
  // Add this state near the other useState declarations
  const [guardrailsList, setGuardrailsList] = useState<string[]>([]);
  const canViewPolicies = useCan("viewPolicies");
  const [policiesList, setPoliciesList] = useState<string[]>([]);
  const [loggingSettings, setLoggingSettings] = useState<any[]>([]);
  const [modelAliases, setModelAliases] = useState<{ [key: string]: string }>({});

  const { data: defaultTeamSettings } = useQuery({
    queryKey: ["defaultTeamSettings"],
    queryFn: () => getDefaultTeamSettings(accessToken as string),
    enabled: isTeamModalVisible && accessToken != null,
    retry: false,
    staleTime: 60_000,
  });
  const defaultBudgetDuration: string | undefined = defaultTeamSettings?.values?.budget_duration ?? undefined;
  const budgetDurationPlaceholder = defaultBudgetDuration
    ? `Default: ${getBudgetDurationLabel(defaultBudgetDuration)} (${defaultBudgetDuration})`
    : "n/a";

  // Add this useEffect to fetch guardrails
  useEffect(() => {
    const fetchGuardrails = async () => {
      try {
        if (accessToken == null) {
          return;
        }

        const response = await getGuardrailsList(accessToken);
        const guardrailNames = response.guardrails.map((g: { guardrail_name: string }) => g.guardrail_name);
        setGuardrailsList(guardrailNames);
      } catch (error) {
        console.error("Failed to fetch guardrails:", error);
      }
    };

    fetchGuardrails();
  }, [accessToken]);

  const openCreateTeamModal = () => {
    // Org admins must scope a team to an org, so with exactly one we preselect it.
    // Proxy admins can create org-less teams, so the field stays optional regardless of org count.
    if (isOrgAdmin && adminOrgs.length === 1) {
      form.setValue("organization_id", adminOrgs[0].organization_id);
    }
    setIsTeamModalVisible(true);
  };

  const selectCreateTeamOrganization = (
    next: string | null,
    currentOrganizationId: string | null,
    onChange: (organizationId: string | null) => void,
  ) => {
    const nextOrganizationId = next;
    if (nextOrganizationId === currentOrganizationId) return;
    onChange(nextOrganizationId);
    form.setValue("models", []);
  };

  const resetCreateForm = () => {
    form.reset(EMPTY_TEAM_CREATE_VALUES);
    setAdditionalSettingsOpen(false);
    setMcpSettingsOpen(false);
    setAgentSettingsOpen(false);
    setSearchToolSettingsOpen(false);
    setLoggingSettings([]);
    setModelAliases({});
  };

  const handleCancel = () => {
    setIsTeamModalVisible(false);
    resetCreateForm();
  };

  const handleDelete = async (team: Team) => {
    // Set the team to delete and open the confirmation modal
    setTeamToDelete(team);
    setIsDeleteModalOpen(true);
  };

  const confirmDelete = async () => {
    if (teamToDelete == null || accessToken == null) {
      return;
    }

    try {
      setIsTeamDeleting(true);
      await teamDeleteCall(accessToken, teamToDelete.team_id);
      await refreshTeams();
      toast.success(t("Team deleted successfully"));
    } catch (error) {
      toast.fromError(t("Error deleting the team: ") + error);
    } finally {
      setIsTeamDeleting(false);
      setIsDeleteModalOpen(false);
      setTeamToDelete(null);
    }
  };

  const cancelDelete = () => {
    setIsDeleteModalOpen(false);
    setTeamToDelete(null);
  };

  useEffect(() => {
    const fetchUserModels = async () => {
      try {
        if (userID === null || userRole === null || accessToken === null) {
          return;
        }
        const models = await fetchAvailableModelsForTeamOrKey(userID, userRole, accessToken);
        if (models) {
          setUserModels(models);
        }
      } catch (error) {
        console.error("Error fetching user models:", error);
      }
    };

    fetchUserModels();
  }, [accessToken, userID, userRole]);

  /** 创建团队；接收已校验表单，提交组织与金额/RPM/TPM，留空速率保持共享；失败展示后台超配原因，成功刷新列表。 */
  const handleCreate = async (formValues: Record<string, any>) => {
    try {
      if (accessToken != null) {
        const organizationId = formValues?.organization_id || currentOrg?.organization_id;
        if (organizationId === "" || typeof organizationId !== "string") {
          formValues.organization_id = null;
        } else {
          formValues.organization_id = organizationId.trim();
        }

        if (formValues.budget_duration === NEVER_RESETS_BUDGET_DURATION) {
          formValues.budget_duration = null;
        }

        toast.info(t("Creating Team"));

        const metadataObject = {
          ...metadataPairsToObject(formValues.metadata),
          ...(loggingSettings.length > 0 ? { logging: loggingSettings.filter((config) => config.callback_name) } : {}),
        };
        formValues.metadata = Object.keys(metadataObject).length > 0 ? JSON.stringify(metadataObject) : undefined;

        if (formValues.secret_manager_settings) {
          if (typeof formValues.secret_manager_settings === "string") {
            if (formValues.secret_manager_settings.trim() === "") {
              delete formValues.secret_manager_settings;
            } else {
              try {
                formValues.secret_manager_settings = JSON.parse(formValues.secret_manager_settings);
              } catch (e) {
                throw new Error(t("Failed to parse secret manager settings: ") + e);
              }
            }
          }
        }

        delete formValues.allowed_vector_store_ids;
        delete formValues.allowed_mcp_servers_and_groups;
        delete formValues.mcp_tool_permissions;
        delete formValues.allowed_mcp_access_groups;
        delete formValues.allowed_agents_and_groups;
        delete formValues.object_permission_search_tools;
        delete formValues.object_permission_skills;
        delete formValues.object_permission;
        delete formValues.policies;

        // Add model_aliases if any are defined
        if (Object.keys(modelAliases).length > 0) {
          formValues.model_aliases = modelAliases;
        }

        await teamCreateCall(accessToken, {
          team_alias: formValues.team_alias,
          team_description: formValues.team_description ?? "",
          organization_id: formValues.organization_id,
          models: Array.isArray(formValues.models) ? formValues.models : [],
          max_budget: formValues.max_budget,
          rpm_limit: formValues.rpm_limit == null || String(formValues.rpm_limit).trim() === "" ? null : Number(formValues.rpm_limit),
          tpm_limit: formValues.tpm_limit == null || String(formValues.tpm_limit).trim() === "" ? null : Number(formValues.tpm_limit),
          ...(formValues.route_template_id ? { route_template_id: formValues.route_template_id } : {}),
        });
        toast.success(t("Team created"));
        await refreshTeams();
        resetCreateForm();
        setIsTeamModalVisible(false);
      }
    } catch (error) {
      console.error("Error creating the team:", error);
      toast.fromError(t("Error creating the team: ") + extractProxyErrorMessage(error));
    }
  };

  const mountedCreateValues = (values: TeamCreateFormValues): Record<string, unknown> => {
    const unmounted = new Set<string>([
      ...(additionalSettingsOpen ? [] : ADDITIONAL_SETTINGS_FIELDS),
      ...(additionalSettingsOpen && canViewPolicies ? [] : ["policies"]),
      ...(mcpSettingsOpen ? [] : MCP_SETTINGS_FIELDS),
      ...(agentSettingsOpen ? [] : AGENT_SETTINGS_FIELDS),
      ...(searchToolSettingsOpen ? [] : SEARCH_TOOL_SETTINGS_FIELDS),
      ...(skillSettingsOpen ? [] : SKILL_SETTINGS_FIELDS),
    ]);
    return Object.fromEntries(Object.entries(values).filter(([key]) => !unmounted.has(key)));
  };

  const onCreateSubmit = (values: TeamCreateFormValues) => handleCreate(mountedCreateValues(values));

  const is_team_admin = (team: any) => {
    if (team == null || team.members_with_roles == null) {
      return false;
    }
    for (let i = 0; i < team.members_with_roles.length; i++) {
      const member = team.members_with_roles[i];
      if (member.user_id == userID && isTeamAdminRole(member.role)) {
        return true;
      }
    }
    return false;
  };

  const tabItems = [
    {
      key: "your-teams",
      label: t("Your Teams"),
      className: "flex min-h-0 flex-1 flex-col",
      children: (
        <>
          <TeamsTable
            userRole={userRole}
            userID={userID}
            onSelectTeam={(team) => {
              setSelectedTeam(team);
              void setSelectedTeamId(team.team_id);
              setEditTeam(false);
            }}
            onEditTeam={(team) => {
              setSelectedTeam(team);
              void setSelectedTeamId(team.team_id);
              setEditTeam(true);
            }}
            onDeleteTeam={handleDelete}
          />

          <DeleteResourceModal
            isOpen={isDeleteModalOpen}
            title={t("Delete Team?")}
            alertMessage={(() => {
              const deleteKeyCount = teamToDelete?.keys_count ?? teamToDelete?.keys?.length ?? 0;
              return deleteKeyCount === 0
                ? undefined
                : t("Warning: This team has {deleteKeyCount} keys associated with it. Deleting the team will also delete all associated keys, along with any models created for this team. This action is irreversible.", { deleteKeyCount });
            })()}
            message={t("Are you sure you want to delete this team, all its keys, and any models created for it? This action cannot be undone.")}
            resourceInformationTitle={t("Team Information")}
            resourceInformation={[
              { label: t("Team ID"), value: teamToDelete?.team_id, code: true },
              { label: t("Team Name"), value: teamToDelete?.team_alias },
              {
                label: t("Keys"),
                value: teamToDelete?.keys_count ?? teamToDelete?.keys?.length ?? 0,
              },
              { label: t("Members"), value: teamToDelete?.members_with_roles?.length },
            ]}
            requiredConfirmation={teamToDelete?.team_alias}
            onCancel={cancelDelete}
            onOk={confirmDelete}
            confirmLoading={isTeamDeleting}
          />
        </>
      ),
    },
    {
      key: "available-teams",
      label: t("Available Teams"),
      className: "min-h-0 flex-1 overflow-y-auto",
      children: <AvailableTeamsPanel accessToken={accessToken} userID={userID} />,
    },

  ];

  return (
    <main className="flex h-full min-h-0 flex-col">
      {selectedTeamId ? (
        <TeamInfoView
          teamId={selectedTeamId}
          onUpdate={() => {
            refreshTeams();
          }}
          onClose={() => {
            setSelectedTeam(null);
            void setSelectedTeamId(null);
            setEditTeam(false);
          }}
          accessToken={accessToken}
          is_team_admin={is_team_admin(selectedTeam?.team_id === selectedTeamId ? selectedTeam : null)}
          is_proxy_admin={userRole == "Admin"}
          userModels={userModels}
          editTeam={editTeam}
          premiumUser={premiumUser}
        />
      ) : (
        <Tabs defaultValue={tabItems[0].key} className="min-h-0 flex-1 gap-6">
          <PageHeader
            icon={<Users />}
            title={t("pages.teams.title")}
            subtitle={t("pages.teams.subtitle")}
            primaryAction={
              canCreateOrManageTeams(userRole, userID, organizations) ? (
                <UIButton onClick={openCreateTeamModal} data-testid="create-team-button">
                  <Plus className="size-4" />
                  {t("pages.teams.create")}
                </UIButton>
              ) : undefined
            }
            tabs={({ leadingControls }) => (
              <TabsList
                variant="line"
                className="gap-0 p-0 [&>[data-slot=tabs-trigger]+[data-slot=tabs-trigger]]:ml-[22px]"
              >
                {leadingControls}
                {tabItems.map((item) => (
                  <TabsTrigger
                    key={item.key}
                    value={item.key}
                    className="flex-none px-0 py-[7px] data-active:font-semibold"
                  >
                    {item.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            )}
          />
          {tabItems.map((item) => (
            <TabsContent key={item.key} value={item.key} className={item.className}>
              {item.children}
            </TabsContent>
          ))}
        </Tabs>
      )}

      {canCreateOrManageTeams(userRole, userID, organizations) && (
        <Dialog open={isTeamModalVisible} onOpenChange={(open) => !open && handleCancel()}>
          <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[1000px]">
            <DialogHeader>
              <DialogTitle>{t("pages.teams.create")}</DialogTitle>
            </DialogHeader>
            <TooltipProvider>
              <form onSubmit={form.handleSubmit(onCreateSubmit)}>
                <FieldGroup>
                  <QuotaGuide scope="team" />
                  <FormField control={form.control} name="team_alias" label={t("Team Name")}>
                    {({ ref, value, ...field }) => (
                      <UIInput {...field} ref={ref} value={value ?? ""} data-testid="team-name-input" />
                    )}
                  </FormField>
                  {(() => {
                    const isSingleOrg = adminOrgs.length === 1;
                    const hasNoOrgs = adminOrgs.length === 0;
                    const soleOrganizationId = isSingleOrg ? adminOrgs[0].organization_id ?? null : null;

                    return (
                      <>
                        <FormField
                          control={form.control}
                          name="organization_id"
                          className="mt-8"
                          label={t("Organization")}
                          description={
                            isOrgAdmin && isSingleOrg
                              ? t("You can only create teams within this organization")
                              : isOrgAdmin
                                ? t("required")
                                : undefined
                          }
                        >
                          {({ id, value, onChange }) => (
                            <SearchSelect
                              inputId={id}
                              value={value ?? ""}
                              options={adminOrgs.map((org) => ({
                                value: org.organization_id ?? "",
                                label: org.organization_alias ?? "",
                                sublabel: org.organization_id ?? "",
                              }))}
                              disabled={isOrgAdmin && soleOrganizationId !== null && value === soleOrganizationId}
                              allowClear={!isOrgAdmin}
                              placeholder={
                                hasNoOrgs ? t("No organizations available") : t("Search or select an Organization")
                              }
                              emptyText={t("No organizations available")}
                              onValueChange={(next) => selectCreateTeamOrganization(next, value ?? null, onChange)}
                            />
                          )}
                        </FormField>

                        {isOrgAdmin && !isSingleOrg && adminOrgs.length > 1 && (
                          <div className="mb-8 rounded-md border border-info/20 bg-info/10 p-4">
                            <span className="text-sm text-info">
                              {t("Please select an organization to create a team for. You can only create teams within organizations where you are an admin.")}
                            </span>
                          </div>
                        )}
                      </>
                    );
                  })()}
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
                        organizationID={watchedOrganizationId ?? undefined}
                        options={{ includeSpecialOptions: false }}
                        context="team"
                        dataTestId="create-team-models-select"
                      />
                    )}
                  </FormField>

                  <FormField control={form.control} name="route_template_id" label={t("pages.routeTemplates.title")}>
                    {({ value, onChange }) => (
                      <RouteTemplateSelect
                        accessToken={accessToken}
                        value={value ?? ""}
                        onChange={onChange}
                        scope="team"
                      />
                    )}
                  </FormField>

                  <FormField control={form.control} name="max_budget" label={t("Max Budget (USD)")} description={t("quotaGuide.team")}>
                    {({ ref, value, ...field }) => (
                      <NumericalInput {...field} ref={ref} value={value ?? ""} step={0.01} precision={2} width={200} />
                    )}
                  </FormField>
                  {(["rpm_limit", "tpm_limit"] as const).map((name) => (
                    <FormField key={name} control={form.control} name={name}
                      label={t(name === "rpm_limit" ? "Requests per minute Limit (RPM)" : "Tokens per minute Limit (TPM)")}
                      description={t("quotaGuide.rates")}>
                      {({ ref, value, ...field }) => (
                        <UIInput {...field} ref={ref} type="number" min="0" max="2147483647" step="1"
                          value={value ?? ""} placeholder={t("quotaGuide.rateBlank")} />
                      )}
                    </FormField>
                  ))}
                </FieldGroup>
                <div className="mt-[10px] text-right">
                  <UIButton type="submit" data-testid="create-team-submit">
                    {t("pages.teams.create")}
                  </UIButton>
                </div>
              </form>
            </TooltipProvider>
          </DialogContent>
        </Dialog>
      )}
    </main>
  );
};

export default Teams;
