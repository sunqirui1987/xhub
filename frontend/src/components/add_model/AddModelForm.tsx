import { useProviderFields } from "@/app/(dashboard)/hooks/providers/useProviderFields";
import { useGuardrails } from "@/app/(dashboard)/hooks/guardrails/useGuardrails";
import { useTags } from "@/app/(dashboard)/hooks/tags/useTags";
import { all_admin_roles, isUserTeamAdminForAnyTeam } from "@/utils/roles";
import { modelCreationScope } from "@/utils/modelPermissions";
import { Switch } from "@/components/ui/switch";
import { Field, FieldLabel } from "@/components/ui/field";
import { Card, CardContent } from "@/components/ui/card";
import { SearchSelect, type SearchSelectOption } from "@/components/shared/SearchSelect";
import { SimpleTooltip } from "@/components/ui/tooltip";
import { Info } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/shared/Alert";
import { Button } from "@/components/ui/button";
import React, { useEffect, useMemo, useState } from "react";
import { FormProvider, useWatch, type UseFormReturn } from "react-hook-form";
import TeamDropdown from "../common_components/team_dropdown";
import { requiredRule } from "../common_components/formRules";
import { labelWithHint } from "@/components/shared/form/LabelWithHint";
import {
  MountedFormField,
  MountedFormProvider,
  type MountRegistry,
  type MountedFormValues,
} from "../common_components/MountedFormField";
import type { Team } from "../key_team_helpers/key_list";
import { type CredentialItem, type ProviderCreateInfo, modelAvailableCall } from "../networking";
import { ProviderLogo } from "../molecules/models/ProviderLogo";
import AccessGroupTagsCombobox from "./AccessGroupTagsCombobox";
import AdvancedSettings from "./advanced_settings";
import ConditionalPublicModelName from "./conditional_public_model_name";
import LiteLLMModelNameField from "./litellm_model_name";
import ConnectionErrorDisplay from "./model_connection_test";
import ProviderSpecificFields from "./provider_specific_fields";
import EndpointTypeField from "./endpoint_type_field";
import ModelPriceFields from "./model_price_fields";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { providerModelSlug } from "../provider_info_helpers";
import { t } from "@/i18n";

interface AddModelFormProps {
  form: UseFormReturn<MountedFormValues>; // For the Add Model tab
  registry: MountRegistry;
  mountedValues: () => MountedFormValues;
  handleOk: () => Promise<boolean>;
  selectedProvider: string | null;
  setSelectedProvider: (provider: string | null) => void;
  providerModels: string[];
  setProviderModelsFn: (provider: string | null) => void;
  getPlaceholder: (provider: string) => string;
  showAdvancedSettings: boolean;
  setShowAdvancedSettings: (show: boolean) => void;
  teams: Team[] | null;
  credentials: CredentialItem[];
  modelCostMap?: Record<string, { endpoint_type?: string }> | null;
}

const connectionTestModelName = (values: MountedFormValues): string | undefined => {
  const named = values.model_name || values.model;
  if (Array.isArray(named)) {
    return named.join(", ");
  }
  return typeof named === "string" ? named : undefined;
};

const AddModelForm: React.FC<AddModelFormProps> = ({
  form,
  registry,
  mountedValues,
  handleOk,
  selectedProvider,
  setSelectedProvider,
  providerModels,
  setProviderModelsFn,
  getPlaceholder,
  showAdvancedSettings,
  setShowAdvancedSettings,
  teams,
  credentials,
  modelCostMap,
}) => {
  const testMode = useWatch({ control: form.control, name: "mode" }) as string | undefined;
  const [isResultModalVisible, setIsResultModalVisible] = useState<boolean>(false);
  const [isTestingConnection, setIsTestingConnection] = useState<boolean>(false);
  // Using a unique ID to force the ConnectionErrorDisplay to remount and run a fresh test
  const [connectionTestId, setConnectionTestId] = useState<string>("");

  const { accessToken, userRole, premiumUser, userId, isViewOnly } = useAuthorized();
  const {
    data: providerMetadata,
    isLoading: isProviderMetadataLoading,
    error: providerMetadataError,
  } = useProviderFields();
  const { data: guardrailsData } = useGuardrails();
  const guardrailsList = guardrailsData?.guardrails.map((g) => g.guardrail_name);
  const { data: tagsList } = useTags();
  const selectedCredentialName = useWatch({ control: form.control, name: "litellm_credential_name" });

  const handleTestConnection = async () => {
    setIsTestingConnection(true);
    setConnectionTestId(`test-${Date.now()}`);
    setIsResultModalVisible(true);
  };

  const [isTeamOnly, setIsTeamOnly] = useState<boolean>(false);
  const [modelAccessGroups, setModelAccessGroups] = useState<string[]>([]);
  // Team admin specific state
  const [teamAdminSelectedTeam, setTeamAdminSelectedTeam] = useState<string | null>(null);

  useEffect(() => {
    const fetchModelAccessGroups = async () => {
      const response = await modelAvailableCall(accessToken, "", "", false, null, true, true);
      setModelAccessGroups(response["data"].map((model: any) => model["id"]));
    };
    fetchModelAccessGroups();
  }, [accessToken]);

  const sortedProviderMetadata: ProviderCreateInfo[] = useMemo(() => {
    if (!providerMetadata) {
      return [];
    }
    return [...providerMetadata].sort((a, b) => a.provider_display_name.localeCompare(b.provider_display_name));
  }, [providerMetadata]);

  const providerOptions: SearchSelectOption[] = useMemo(
    () =>
      sortedProviderMetadata.map((providerInfo) => ({
        label: providerInfo.provider_display_name,
        value: providerInfo.provider,
        icon: <ProviderLogo provider={providerInfo.provider} className="w-5 h-5" />,
      })),
    [sortedProviderMetadata],
  );

  const credentialOptions: SearchSelectOption[] = useMemo(
    () => [
      { label: t("None"), value: "" },
      ...credentials.map((credential) => ({
        label: credential.credential_name,
        value: credential.credential_name,
      })),
    ],
    [credentials],
  );

  const formProvider = useWatch({ control: form.control, name: "custom_llm_provider" });
  const selectedProviderName = typeof formProvider === "string" && formProvider !== "" ? formProvider : null;

  // The price map can arrive after the provider is chosen. Re-filter whenever
  // either side changes, and match on litellm_provider so Aliyun lists Qwen.
  useEffect(() => {
    setSelectedProvider(selectedProviderName);
    setProviderModelsFn(providerModelSlug(selectedProviderName, providerMetadata));
  }, [selectedProviderName, providerMetadata, modelCostMap, setSelectedProvider, setProviderModelsFn]);

  const applyProviderSelection = (provider: string | null) => {
    setSelectedProvider(provider);
    setProviderModelsFn(providerModelSlug(provider, providerMetadata));
    form.setValue("model", []);
    form.setValue("model_name", undefined);
    form.setValue("model_mappings", []);
  };

  const providerMetadataErrorText = providerMetadataError
    ? providerMetadataError instanceof Error
      ? providerMetadataError.message
      : "Failed to load providers"
    : null;

  const isAdmin = all_admin_roles.includes(userRole);
  const isTeamAdmin = isUserTeamAdminForAnyTeam(teams, userId);
  // Same owner the Auto-Routers tab uses, so the two creation forms cannot disagree about
  // who has to name a team. This form is only reachable when creation is allowed at all.
  const createScope = modelCreationScope(
    { userRole, userID: userId, isViewOnly },
    { teams, disabledForInternalUsers: false },
  );
  const requiresTeamScope = createScope === "team-required";

  return (
    <>
      <h2 className="mb-4 text-2xl font-semibold text-foreground">{t("Add Model")}</h2>

      <Card>
        <CardContent>
          <FormProvider {...form}>
            <MountedFormProvider value={{ control: form.control, registry }}>
              <form
                onSubmit={(event) => {
                  event.preventDefault();
                  void handleOk().then((submitted) => {
                    if (submitted) {
                      setTeamAdminSelectedTeam(null);
                    }
                  });
                }}
              >
                <>
                  {requiresTeamScope && (
                    <>
                      <MountedFormField
                        label={labelWithHint(t("Select Team"), t("Select the team for which you want to add this model"))}
                        name="team_id"
                        required
                        rules={{ validate: { required: requiredRule(t("Please select a team to continue")) } }}
                        className="mb-4"
                      >
                        {(control) => (
                          <TeamDropdown
                            value={control.value as string | undefined}
                            onChange={(value) => {
                              control.onChange(value);
                              setTeamAdminSelectedTeam(value);
                            }}
                          />
                        )}
                      </MountedFormField>
                      {!teamAdminSelectedTeam && (
                        <Alert variant="info" className="mb-4">
                          <Info />
                          <AlertTitle>{t("Team Selection Required")}</AlertTitle>
                          <AlertDescription>
                            {t("As a team admin, you need to select your team first before adding models.")}
                          </AlertDescription>
                        </Alert>
                      )}
                    </>
                  )}
                  {(isAdmin || (isTeamAdmin && teamAdminSelectedTeam)) && (
                    <>
                      <MountedFormField
                        label={labelWithHint(t("Provider"), t("E.g. OpenAI, Azure OpenAI, Anthropic, Bedrock, etc."))}
                        name="custom_llm_provider"
                        required
                        rules={{ validate: { required: requiredRule("Required") } }}
                        className="mb-4"
                      >
                        {(control) => (
                          <SearchSelect
                            inputId={control.id}
                            options={providerOptions}
                            emptyText={providerMetadataErrorText ?? t("No providers found")}
                            placeholder={isProviderMetadataLoading ? t("Loading providers...") : t("Select a provider")}
                            value={typeof control.value === "string" ? control.value : null}
                            onValueChange={(value) => {
                              control.onChange(value);
                              applyProviderSelection(value);
                            }}
                          />
                        )}
                      </MountedFormField>
                      <LiteLLMModelNameField
                        selectedProvider={selectedProvider}
                        providerModels={providerModels}
                        modelCostMap={modelCostMap}
                        getPlaceholder={getPlaceholder}
                      />

                      {/* Conditionally Render "Public Model Name" */}
                      <ConditionalPublicModelName />

                      <EndpointTypeField selectedProvider={selectedProvider} modelCostMap={modelCostMap} />

                      {/* 单价：由上面选中的模型从价目表带出，可改。
                          它是必答的——网关按模型名取价，名字对不上价目表时这次调用
                          记一行零费用，不报错也不告警。所以不能静默留空。 */}
                      <ModelPriceFields
                        selectedProvider={selectedProvider}
                        modelCostMap={modelCostMap}
                      />
                    </>
                  )}
                  <div className="flex justify-end items-center mb-4">
                    <div className="space-x-2">
                      <Button
                        variant="outline"
                        data-testid="test-connect-btn"
                        onClick={handleTestConnection}
                        disabled={isTestingConnection}
                        aria-busy={isTestingConnection}
                      >
                        {t("Test Connect")}
                      </Button>
                      <Button data-testid="add-model-btn" type="submit">
                        {t("Add Model")}
                      </Button>
                    </div>
                  </div>
                </>
              </form>
            </MountedFormProvider>
          </FormProvider>
        </CardContent>
      </Card>

      {/* Test Connection Results Modal */}
      <Dialog
        open={isResultModalVisible}
        onOpenChange={(open) => {
          if (!open) {
            setIsResultModalVisible(false);
            setIsTestingConnection(false);
          }
        }}
      >
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[700px]">
          <DialogHeader>
            <DialogTitle>{t("Connection Test Results")}</DialogTitle>
          </DialogHeader>
          {/* Only render the ConnectionErrorDisplay when modal is visible and we have a test ID */}
          {isResultModalVisible && (
            <ConnectionErrorDisplay
              // The key prop tells React to create a fresh component instance when it changes
              key={connectionTestId}
              formValues={mountedValues()}
              accessToken={accessToken}
              testMode={testMode ?? ""}
              modelName={connectionTestModelName(form.getValues())}
              onClose={() => {
                setIsResultModalVisible(false);
                setIsTestingConnection(false);
              }}
              onTestComplete={() => setIsTestingConnection(false)}
            />
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setIsResultModalVisible(false);
                setIsTestingConnection(false);
              }}
            >
              {t("Close")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default AddModelForm;
