"use client";

import { useCallback, useState } from "react";
import { useForm } from "react-hook-form";
import { useQueryClient } from "@tanstack/react-query";
import AddModelForm from "@/components/add_model/AddModelForm";
import { handleAddModelSubmit } from "@/components/add_model/handle_add_model_submit";
import {
  projectMountedValues,
  useMountRegistry,
  type MountedFormValues,
} from "@/components/common_components/MountedFormField";
import { Providers, getPlaceholder, getProviderModels } from "@/components/provider_info_helpers";
import { useModelCostMap } from "@/app/(dashboard)/hooks/models/useModelCostMap";
import { useCredentials } from "@/app/(dashboard)/hooks/credentials/useCredentials";
import { useTeams } from "@/app/(dashboard)/hooks/teams/useTeams";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { t } from "@/i18n";

const INITIAL_VALUES: MountedFormValues = { litellm_credential_name: null };

/** 表单上凡是能作为计费依据的字段。按 token、按张、按秒、按次各算一类。 */
const PRICE_FIELDS = [
  "input_cost_per_token",
  "output_cost_per_token",
  "input_cost_per_token_peak",
  "output_cost_per_token_peak",
  "cache_read_input_token_cost",
  "cache_creation_input_token_cost",
  "input_cost_per_image",
  "output_cost_per_image",
  "input_cost_per_second",
  "output_cost_per_second",
  "search_context_cost_per_query",
] as const;

/**
 * 这条模型会被按什么价计费。
 *
 * 价目表里有它（有扁平费率字段，或者有 rates 数组），或者表单上填了至少一个
 * 单价，就算定价了。两者都没有时返回假：网关取不到价，这次调用会记成零费用。
 *
 * 参数 values（MountedFormValues）：表单当前值；costMap（Record<string, ...> | undefined）：价目表。
 * 返回 bool（bool）：这次调用会被计费时为真。
 */
function modelPriced(
  values: MountedFormValues,
  costMap: Record<string, Record<string, unknown>> | undefined,
): boolean {
  const picked = Array.isArray(values.model) ? values.model[0] : values.model;
  if (typeof picked === "string" && picked !== "") {
    const row = costMap?.[picked];
    if (row) {
      if (Array.isArray(row.rates) && row.rates.length > 0) return true;
      for (const key of Object.keys(row)) {
        if (key.includes("cost") && typeof row[key] === "number") return true;
      }
    }
  }
  const filled = (value: unknown) => value !== undefined && value !== null && value !== "";
  return PRICE_FIELDS.some((field) => filled(values[field]));
}

export default function AddModelPanel() {
  const { accessToken } = useAuthorized();
  const form = useForm<MountedFormValues>({ mode: "onChange", defaultValues: INITIAL_VALUES });
  const registry = useMountRegistry();
  const queryClient = useQueryClient();
  const { data: modelCostMapData } = useModelCostMap();
  const { data: credentialsResponse } = useCredentials();
  const { data: teams } = useTeams();
  const [selectedProvider, setSelectedProvider] = useState<string | null>(Providers.Anthropic);
  const [providerModels, setProviderModels] = useState<string[]>([]);
  const [showAdvancedSettings, setShowAdvancedSettings] = useState(false);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["models", "list"] });
  const setProviderModelsFn = useCallback(
    (provider: string | null) => {
      setProviderModels((current) => {
        const next = provider === null ? [] : getProviderModels(provider, modelCostMapData);
        if (current.length === next.length && current.every((model, index) => model === next[index])) {
          return current;
        }
        return next;
      });
    },
    [modelCostMapData],
  );

  const mountedValues = () => projectMountedValues(registry, form.getValues);

  const handleOk = async (): Promise<boolean> => {
    const isValid = await form.trigger(registry.mountedNames() as string[]);
    if (!isValid) {
      return false;
    }
    // 目录里没有这条模型、又没填任何单价时挡住保存。
    //
    // 这不是界面洁癖：网关按模型名取价，取不到就记一行零费用，不报错也不告警。
    // 一个名字写错、价格留空的模型会安安静静地跑，账单上是零——那是客户最后才
    // 发现的那种问题。所以宁可在这里拦住。
    if (!modelPriced(form.getValues(), modelCostMapData)) {
      form.setError("input_cost_per_token", {
        type: "validate",
        message: t("This model is not in the price catalog. Fill in its price, or calls are recorded at zero cost."),
      });
      return false;
    }
    await handleAddModelSubmit(
      mountedValues(),
      accessToken,
      { resetFields: () => form.reset(INITIAL_VALUES) },
      refresh,
    );
    return true;
  };

  return (
    <AddModelForm
      form={form}
      registry={registry}
      mountedValues={mountedValues}
      handleOk={handleOk}
      selectedProvider={selectedProvider}
      setSelectedProvider={setSelectedProvider}
      providerModels={providerModels}
      setProviderModelsFn={setProviderModelsFn}
      getPlaceholder={getPlaceholder}
      showAdvancedSettings={showAdvancedSettings}
      setShowAdvancedSettings={setShowAdvancedSettings}
      teams={teams ?? null}
      credentials={credentialsResponse?.credentials || []}
      modelCostMap={modelCostMapData as Record<string, { endpoint_type?: string }> | undefined}
    />
  );
}
