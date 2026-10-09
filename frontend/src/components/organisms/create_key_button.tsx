"use client";
import { keyKeys } from "@/app/(dashboard)/hooks/keys/useKeys";
import { useSessionIdentity } from "@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { formatNumberWithCommas } from "@/utils/dataUtils";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { SimpleTooltip } from "@/components/ui/tooltip";
import { MultiSelect, type MultiSelectOption } from "@/components/shared/MultiSelect";
import { Info } from "lucide-react";
import React, { useEffect, useMemo, useState } from "react";
import { useForm, useWatch } from "react-hook-form";
import { rolesWithWriteAccess } from "../../utils/roles";
import { t } from "@/i18n";
import BudgetDurationDropdown from "../common_components/budget_duration_dropdown";
import KeyLifecycleSettings from "../common_components/KeyLifecycleSettings";
import {
  MountedFormField,
  MountedFormProvider,
  projectMountedValues,
  useMountRegistry,
  type MountedFormValues,
} from "../common_components/MountedFormField";
import RouteTemplateSelect from "../route_templates/RouteTemplateSelect";
import {
  excludeProxyWideSentinel,
  getModelDisplayName,
  hasAllModelsSentinel,
} from "../key_team_helpers/fetch_available_models_team_key";
import { Team } from "../key_team_helpers/key_list";
import { toast } from "@/lib/toast";
import {
  keyCreateCall,
  keyCreateServiceAccountCall,
  modelAvailableCall,
} from "../networking";
import CreatedKeyDisplay from "../shared/CreatedKeyDisplay";
import NumericalInput from "../shared/numerical_input";
import { buildKeyCreatePayload, type KeyCreateInput } from "./createKeyPayload";
import { simplifyKeyGenerateError } from "./utils";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

const KEY_TYPE_OPTIONS = [
  { value: "llm_api", label: t("AI APIs"), hint: t("Can call only AI API routes (chat/completions, embeddings, etc.)") },
  { value: "management", label: t("Management"), hint: t("Can call only management routes (user/team/key management)") },
  { value: "default", label: t("Full Access"), hint: t("Can call all routes (AI APIs, Management, and read-only)") },
];

type FieldWrite = (value: unknown) => void;


const isBlank = (value: unknown): boolean => value === undefined || value === null || value === "";

const requiredRule = (required: boolean, message: string) => ({
  validate: (value: unknown) => (required && isBlank(value) ? message : true),
});

const ceilingRule = (ceiling: number | null | undefined, message: (limit: number) => string) => ({
  validate: (value: unknown) =>
    value && ceiling !== null && ceiling !== undefined && (value as number) > ceiling ? message(ceiling) : true,
});

/**
 * Interface for pre-filling the create key form from URL parameters
 */
export interface CreateKeyPrefillData {
  owned_by?: "you" | "service_account" | "another_user";
  team_id?: string;
  key_alias?: string;
  models?: string[];
  key_type?: "default" | "llm_api" | "management";
}

interface CreateKeyProps {
  team: Team | null;
  data: any[] | null;
  teams: Team[] | null;
  addKey: (data: any) => void;
  autoOpenCreate?: boolean;
  prefillData?: CreateKeyPrefillData;
}

export const fetchTeamModels = async (
  userID: string,
  userRole: string,
  accessToken: string,
  teamID: string | null,
): Promise<string[]> => {
  try {
    if (userID === null || userRole === null) {
      return [];
    }

    if (accessToken !== null) {
      const model_available = await modelAvailableCall(accessToken, userID, userRole, true, teamID, true);
      let available_model_names = model_available["data"].map((element: { id: string }) => element.id);
      return available_model_names;
    }
    return [];
  } catch (error) {
    console.error("Error fetching user models:", error);
    return [];
  }
};

export const fetchUserModels = async (
  userID: string,
  userRole: string,
  accessToken: string,
  setUserModels: (models: string[]) => void,
) => {
  try {
    if (userID === null || userRole === null) {
      return;
    }

    if (accessToken !== null) {
      const model_available = await modelAvailableCall(accessToken, userID, userRole);
      let available_model_names = model_available["data"].map((element: { id: string }) => element.id);
      setUserModels(available_model_names);
    }
  } catch (error) {
    console.error("Error fetching user models:", error);
  }
};

/**
 * ─────────────────────────────────────────────────────────────────────────
 * @deprecated
 * This component is being DEPRECATED in favor of src/app/(dashboard)/virtual-keys/components/CreateKey.tsx
 * Please contribute to the new refactor.
 * ─────────────────────────────────────────────────────────────────────────
 */
const CreateKey: React.FC<CreateKeyProps> = ({ team, teams, data, addKey, autoOpenCreate, prefillData }) => {
  const { accessToken, userId: userID, userRole } = useAuthorized();
  const { data: identity } = useSessionIdentity();
  const queryClient = useQueryClient();
  const [formDefaults] = useState<MountedFormValues>(() => ({
    team_id: team ? team.team_id : null,
    key_type: "llm_api",
    duration: "",
  }));
  const form = useForm<MountedFormValues>({
    mode: "onChange",
    shouldUnregister: false,
    defaultValues: formDefaults,
  });
  const registry = useMountRegistry();
  const mountedForm = useMemo(() => ({ control: form.control, registry }), [form.control, registry]);
  const [isModalVisible, setIsModalVisible] = useState(false);
  const [apiKey, setApiKey] = useState(null);
  const [modelsToPick, setModelsToPick] = useState<string[]>([]);
  const keyOwner = "you";
  const [hasPrefilled, setHasPrefilled] = useState(false);
  const [pendingPrefillModels, setPendingPrefillModels] = useState<string[] | null>(null);
  const [selectedCreateKeyTeam, setSelectedCreateKeyTeam] = useState<Team | null>(team);
  const [keyType, setKeyType] = useState<string>("llm_api");
  const [autoRotationEnabled, setAutoRotationEnabled] = useState<boolean>(false);
  const [rotationInterval, setRotationInterval] = useState<string>("30d");
  const [routeTemplateId, setRouteTemplateId] = useState("");
  const selectedModels: string[] = (useWatch({ control: form.control, name: "models" }) as string[] | undefined) ?? [];
  const handleCancel = () => {
    setIsModalVisible(false);
    setApiKey(null);
    setSelectedCreateKeyTeam(null);
    form.reset(formDefaults);
    setKeyType("llm_api");
    setAutoRotationEnabled(false);
    setRotationInterval("30d");
    setRouteTemplateId("");
  };

  // A key belongs to the signed-in user and one team they already belong to.
  // The account is not asked to pick an owner, an organization, or any team
  // outside that membership.
  const memberTeams = useMemo(() => {
    if (!teams?.length || !identity) return [];
    const membershipIds = new Set(identity.teams?.map((item) => item.team_id) ?? []);
    return teams.filter((item) => membershipIds.has(item.team_id));
  }, [teams, identity]);

  useEffect(() => {
    if (memberTeams.length !== 1) return;
    const only = memberTeams[0];
    if (selectedCreateKeyTeam?.team_id === only.team_id) return;
    setSelectedCreateKeyTeam(only);
    form.setValue("team_id", only.team_id);
  }, [memberTeams, selectedCreateKeyTeam, form]);

  // Auto-open modal and prefill form from URL params (deep link).
  // Guarded by write access so we don't open for read-only users.
  useEffect(() => {
    if (autoOpenCreate && !hasPrefilled && teams && userRole && rolesWithWriteAccess.includes(userRole)) {
      // Open the modal
      setIsModalVisible(true);
      setHasPrefilled(true);

      // Apply prefill data if provided
      if (prefillData) {
        // Set team - find the team by ID and set it (only if team exists in user's teams)
        if (prefillData.team_id) {
          const selectedTeam = teams?.find((t) => t.team_id === prefillData.team_id) || null;
          if (selectedTeam) {
            setSelectedCreateKeyTeam(selectedTeam);
            form.setValue("team_id", prefillData.team_id);
          }
          // Silently ignore invalid team_id - don't prefill with a team user doesn't have access to
        }

        // Set key alias
        if (prefillData.key_alias) {
          form.setValue("key_alias", prefillData.key_alias);
        }

        // Defer model selection until we load the allowed model list.
        if (prefillData.models && prefillData.models.length > 0) {
          setPendingPrefillModels(prefillData.models);
        }

        // Set key type
        if (prefillData.key_type) {
          setKeyType(prefillData.key_type);
          form.setValue("key_type", prefillData.key_type);
        }
      }
    }
  }, [autoOpenCreate, prefillData, teams, hasPrefilled, form, userRole]);

  // Check if team selection is required
  const isTeamSelectionRequired = modelsToPick.includes("no-default-models");
  const isFormDisabled = isTeamSelectionRequired && !selectedCreateKeyTeam;

  const handleCreate = async (formValues: MountedFormValues) => {
    try {
      const input: KeyCreateInput = {
        formValues: {
          ...formValues,
          team_id: selectedCreateKeyTeam?.team_id ?? formValues.team_id ?? null,
        },
        existingKeys: data,
        keyOwner,
        userID,
        selectedAgentId: null,
        autoRotationEnabled,
        rotationInterval,
        routeTemplateId,
      };
      const built = buildKeyCreatePayload(input);
      if (built.kind === "duplicate_alias") {
        throw new Error(
          t("Key alias {value0} already exists for team with ID {value1}, please provide another key alias", { value0: (built.alias), value1: (built.teamId) }),
        );
      }

      toast.info(t("Making API Call"));
      setIsModalVisible(true);

      if (built.kind === "agent_not_selected") {
        toast.fromError(t("Please select an agent"));
        return;
      }
      const { payload, endpoint } = built;

      const response =
        endpoint === "service_account"
          ? await keyCreateServiceAccountCall(accessToken, payload)
          : await keyCreateCall(accessToken, userID, payload);

      // Add the data to the state in the parent component
      // Also directly update the keys list in VirtualKeysTable without an API call
      addKey(response);

      // Invalidate and refetch all keys list queries to update the table
      // This will trigger a refetch of all key list queries regardless of pagination
      queryClient.invalidateQueries({ queryKey: keyKeys.lists() });

      setApiKey(response["key"]);
      toast.success(t("Virtual Key Created"));
      form.reset(formDefaults);
      localStorage.removeItem("userData" + userID);
    } catch (error) {
      const simplifiedError = simplifyKeyGenerateError(error);
      toast.fromError(simplifiedError);
    }
  };

  const handleSubmit = (event: React.FormEvent<HTMLFormElement>) =>
    void form.handleSubmit(() => handleCreate(projectMountedValues(registry, form.getValues)))(event);

  // Fetch available models when team or auth changes.
  // Note: Model prefill from URL params is handled by the useEffect below, which
  // watches for pendingPrefillModels + modelsToPick to both be populated.
  useEffect(() => {
    if (userID && userRole && accessToken) {
      fetchTeamModels(userID, userRole, accessToken, selectedCreateKeyTeam?.team_id ?? null).then((models) => {
        const allModels = excludeProxyWideSentinel(
          Array.from(new Set([...(selectedCreateKeyTeam?.models ?? []), ...models])),
        );
        setModelsToPick(allModels);
      });
    }
    // Only clear models if we don't have pending prefill models
    if (!pendingPrefillModels) {
      form.setValue("models", []);
    }
  }, [selectedCreateKeyTeam, accessToken, userID, userRole, form]);

  // Apply deferred model prefill once the available model list arrives.
  // This handles timing where prefill data arrives before or after models are fetched.
  useEffect(() => {
    if (!pendingPrefillModels || pendingPrefillModels.length === 0) {
      return;
    }
    if (!modelsToPick || modelsToPick.length === 0) {
      return;
    }

    const validModels = pendingPrefillModels.filter((model) => modelsToPick.includes(model));
    if (validModels.length > 0) {
      form.setValue("models", validModels);
    }
    setPendingPrefillModels(null);
  }, [pendingPrefillModels, modelsToPick, form]);

  const chooseMemberTeam = (teamId: string) => {
    const chosen = memberTeams.find((item) => item.team_id === teamId) ?? null;
    setSelectedCreateKeyTeam(chosen);
    form.setValue("team_id", teamId);
  };

  const modelOptions: MultiSelectOption[] = [
    ...(selectedCreateKeyTeam
      ? [{ value: "all-team-models", label: t("All Team Models") }]
      : []),
    ...(!selectedCreateKeyTeam
      ? [{ value: "all-proxy-models", label: t("All Proxy Models") }]
      : []),
    ...modelsToPick.map((model) => ({
      value: model,
      label: getModelDisplayName(model),
      disabled: hasAllModelsSentinel(selectedModels),
    })),
  ];

  const changeKeyType = (write: FieldWrite) => (value: string) => {
    write(value);
    setKeyType(value);
    // Clear models field and disable if management or read_only
    if (value === "management" || value === "read_only") {
      form.setValue("models", []);
    }
  };

  return (
    <div>
      {userRole && rolesWithWriteAccess.includes(userRole) && (
        <Button className="mx-auto" onClick={() => setIsModalVisible(true)} data-testid="create-key-button">
          + {t("pages.apiKeys.create")}
        </Button>
      )}
      <Dialog open={isModalVisible} onOpenChange={(open) => !open && handleCancel()}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[1000px]">
          <DialogHeader>
            <DialogTitle className="text-xl font-semibold text-foreground">{t("pages.apiKeys.create")}</DialogTitle>
          </DialogHeader>
          <MountedFormProvider value={mountedForm}>
            <form onSubmit={handleSubmit}>
              {memberTeams.length > 1 && (
                <div className="mb-8">
                  <MountedFormField label={t("Team")} name="team_id">
                    {(control) => (
                      <Select
                        items={memberTeams.map((item) => ({ value: item.team_id, label: item.team_alias || item.team_id }))}
                        value={typeof control.value === "string" ? control.value : ""}
                        onValueChange={(value) => chooseMemberTeam(String(value))}
                      >
                        <SelectTrigger id={control.id} aria-label={t("Team")}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {memberTeams.map((item) => (
                            <SelectItem key={item.team_id} value={item.team_id}>
                              {item.team_alias || item.team_id}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  </MountedFormField>
                </div>
              )}

              {/* Show message when team selection is required */}
              {isFormDisabled && (
                <div className="mb-8 p-4 bg-info/10 border border-info/20 rounded-md">
                  <p className="text-info text-sm">
                    {t("Please select a team to continue configuring your Virtual Key. If you do not see any teams, please contact your Proxy Admin to either provide you with access to models or to add you to a team.")}
                  </p>
                </div>
              )}

              {/* Section 2: Key Details */}
              {!isFormDisabled && (
                <div className="mb-8">
                  <h3 className="text-lg font-medium text-foreground mb-4">{t("Key Details")}</h3>
                  <MountedFormField
                    label={
                      <span>
                        {t("Key Name")}{" "}
                        <SimpleTooltip content={t("A descriptive name to identify this key")}>
                          <Info className="ml-1 inline size-3.5 align-text-bottom" />
                        </SimpleTooltip>
                      </span>
                    }
                    name="key_alias"
                    required
                    rules={requiredRule(
                      true,
                      t("Please input a {value0}", { value0: t("key name") }),
                    )}
                    help={t("required")}
                  >
                    {(control) => <Input {...control} value={(control.value as string | undefined) ?? ""} />}
                  </MountedFormField>

                  <MountedFormField
                    label={
                      <span>
                        {t("Models")}{" "}
                        <SimpleTooltip content={t("Select which models this key can access. Choose 'All Team Models' to grant access to all models available to the team. Leave empty to allow access to all models.")}>
                          <Info className="ml-1 inline size-3.5 align-text-bottom" />
                        </SimpleTooltip>
                      </span>
                    }
                    name="models"
                    help={
                      keyType === "management" || keyType === "read_only"
                        ? t("Models field is disabled for this key type")
                        : t("optional - leave empty to allow access to all models")
                    }
                    className="mt-4"
                  >
                    {(control) => (
                      <MultiSelect
                        id={control.id}
                        options={modelOptions}
                        value={(control.value as string[] | undefined) ?? []}
                        placeholder={t("Select models")}
                        disabled={keyType === "management" || keyType === "read_only"}
                        onValueChange={(values) => {
                          control.onChange(values);
                          if (values.includes("all-team-models")) {
                            form.setValue("models", ["all-team-models"]);
                          } else if (values.includes("all-proxy-models")) {
                            form.setValue("models", ["all-proxy-models"]);
                          }
                        }}
                      />
                    )}
                  </MountedFormField>

                  <MountedFormField
                    label={
                      <span>
                        {t("Key Type")}{" "}
                        <SimpleTooltip content={t("Select the type of key to determine what routes and operations this key can access")}>
                          <Info className="ml-1 inline size-3.5 align-text-bottom" />
                        </SimpleTooltip>
                      </span>
                    }
                    name="key_type"
                    className="mt-4"
                  >
                    {(control) => (
                      <Select
                        items={KEY_TYPE_OPTIONS}
                        value={control.value as string | undefined}
                        onValueChange={(value: string | null) =>
                          value != null && changeKeyType(control.onChange)(value)
                        }
                      >
                        <SelectTrigger
                          id={control.id}
                          className="w-full"
                          aria-invalid={control["aria-invalid"]}
                          aria-describedby={control["aria-describedby"]}
                        >
                          <SelectValue placeholder={t("Select key type")} />
                        </SelectTrigger>
                        <SelectContent>
                          {KEY_TYPE_OPTIONS.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              <div className="py-1">
                                <div className="font-medium">{option.label}</div>
                                <div className="mt-0.5 text-[11px] text-muted-foreground">{option.hint}</div>
                              </div>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  </MountedFormField>
                </div>
              )}

              {/* Key controls */}
              {!isFormDisabled && (
                <div className="mb-8 space-y-6">
                  <div>
                    <h3 className="mb-4 text-lg font-medium text-foreground">{t("Budget and Routing")}</h3>
                    <div className="grid gap-5 rounded-lg border p-4 sm:grid-cols-2">
                      <MountedFormField
                        label={
                          <span>
                            {t("Max Budget (USD)")}{" "}
                            <SimpleTooltip
                              content={t(
                                "Maximum amount in USD this key can spend. When reached, the key will be blocked from making further requests",
                              )}
                            >
                              <Info className="ml-1 inline size-3.5 align-text-bottom" />
                            </SimpleTooltip>
                          </span>
                        }
                        name="max_budget"
                        help={t("Budget cannot exceed team max budget: ${value0}", {
                          value0:
                            selectedCreateKeyTeam?.max_budget !== null &&
                            selectedCreateKeyTeam?.max_budget !== undefined
                              ? selectedCreateKeyTeam.max_budget
                              : t("unlimited"),
                        })}
                        rules={ceilingRule(selectedCreateKeyTeam?.max_budget, (limit) =>
                          t("Budget cannot exceed team max budget: ${value0}", {
                            value0: formatNumberWithCommas(limit, 4),
                          }),
                        )}
                      >
                        {(control) => (
                          <NumericalInput
                            {...control}
                            value={control.value as number | string | undefined}
                            step={0.01}
                            precision={2}
                            width={200}
                          />
                        )}
                      </MountedFormField>

                      <MountedFormField
                        label={
                          <span>
                            {t("Reset Budget")}{" "}
                            <SimpleTooltip
                              content={t(
                                "How often the budget should reset. For example, setting 'daily' will reset the budget every 24 hours",
                              )}
                            >
                              <Info className="ml-1 inline size-3.5 align-text-bottom" />
                            </SimpleTooltip>
                          </span>
                        }
                        name="budget_duration"
                        help={t("Team Reset Budget: {value0}", {
                          value0:
                            selectedCreateKeyTeam?.budget_duration !== null &&
                            selectedCreateKeyTeam?.budget_duration !== undefined
                              ? selectedCreateKeyTeam.budget_duration
                              : t("None"),
                        })}
                      >
                        {(control) => (
                          <BudgetDurationDropdown
                            id={control.id}
                            value={control.value as string | null | undefined}
                            showNeverResets
                            placeholder={t("Not set")}
                            onChange={(next) => control.onChange(next ?? undefined)}
                          />
                        )}
                      </MountedFormField>

                      <div className="sm:col-span-2">
                        <p className="mb-2 text-sm font-medium">{t("pages.routeTemplates.title")}</p>
                        <RouteTemplateSelect
                          accessToken={accessToken}
                          value={routeTemplateId}
                          onChange={setRouteTemplateId}
                          scope="key"
                        />
                      </div>
                    </div>
                  </div>

                  <div>
                    <h3 className="mb-4 text-lg font-medium text-foreground">{t("Key Lifecycle")}</h3>
                    <div className="rounded-lg border p-4">
                      <MountedFormField name="duration" bare>
                        {(control) => (
                          <KeyLifecycleSettings
                            id={control.id}
                            value={control.value as string | undefined}
                            onChange={control.onChange}
                            autoRotationEnabled={autoRotationEnabled}
                            onAutoRotationChange={setAutoRotationEnabled}
                            rotationInterval={rotationInterval}
                            onRotationIntervalChange={setRotationInterval}
                            isCreateMode
                          />
                        )}
                      </MountedFormField>
                    </div>
                  </div>
                </div>
              )}

              <div style={{ textAlign: "right", marginTop: "10px" }}>
                <Button type="submit" disabled={isFormDisabled}>
                  {t("pages.apiKeys.createSubmit")}
                </Button>
              </div>
            </form>
          </MountedFormProvider>
        </DialogContent>
      </Dialog>

      {apiKey && (
        <Dialog open={isModalVisible} onOpenChange={(open) => !open && handleCancel()}>
          <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
            <div className="grid grid-cols-1 gap-2 w-full">
              <DialogTitle className="text-lg font-medium text-foreground">{t("pages.apiKeys.saveKey")}</DialogTitle>
              {apiKey != null ? (
                <CreatedKeyDisplay apiKey={apiKey} />
              ) : (
                <p className="text-sm">{t("Key being created, this might take 30s")}</p>
              )}
            </div>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
};

export default CreateKey;
