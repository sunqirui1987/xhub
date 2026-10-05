// fetch_models.ts

import { excludeProxyWideSentinel } from "@/components/key_team_helpers/fetch_available_models_team_key";
import { modelAvailableCall, userAvailableModelsCall } from "@/components/networking";

export interface ModelGroup {
  model_group: string;
  mode?: string;
  supports_reasoning?: boolean;
  supported_reasoning_efforts?: string[] | null;
}

interface AvailableModel {
  model_group?: string | null;
  model_name?: string | null;
  id?: string | null;
  role?: string | null;
  model_info?: {
    role?: string | null;
  } | null;
  mode?: string | null;
  category?: string | null;
  capabilities?: string[] | null;
  supports_reasoning?: boolean | null;
  supported_reasoning_efforts?: string[] | null;
}

const isProviderShell = (item: AvailableModel): boolean =>
  item.role === "provider" || item.model_info?.role === "provider";

const toModelGroup = (item: AvailableModel): ModelGroup => {
  const groupName = (item.model_group || item.id || item.model_name) ?? "";
  return {
    model_group: groupName,
    ...(item.mode && { mode: item.mode }),
    ...((item.supports_reasoning === true || item.capabilities?.includes("reasoning")) && { supports_reasoning: true }),
    ...(item.supported_reasoning_efforts !== undefined && {
      supported_reasoning_efforts: item.supported_reasoning_efforts,
    }),
  };
};

export const fetchAvailableModelsForTeam = async (accessToken: string, teamId: string): Promise<ModelGroup[]> => {
  const response = await modelAvailableCall(accessToken, "", "", false, teamId);
  const modelNames: string[] = (response?.data ?? []).map((model: { id: string }) => model.id);

  return excludeProxyWideSentinel(Array.from(new Set(modelNames)))
    .sort((a, b) => a.localeCompare(b))
    .map((model) => ({ model_group: model }));
};

/**
 * Models the signed-in caller may use. This reads /model/available, which any
 * session can call. The management catalog is a different route.
 */
export const fetchAvailableModels = async (accessToken: string): Promise<ModelGroup[]> => {
  try {
    const fetchedModels = await userAvailableModelsCall(accessToken);
    const fetchedData: unknown = fetchedModels?.data;
    const models: ModelGroup[] = (Array.isArray(fetchedData) ? fetchedData : [])
      .filter((item): item is AvailableModel => typeof item === "object" && item !== null && !isProviderShell(item as AvailableModel))
      .map((item) => {
        const group = toModelGroup(item);
        const mode = item.category || item.mode;
        return { ...group, ...(mode && mode !== "other" ? { mode } : { mode: "chat" }) };
      })
      .filter((model: ModelGroup) => model.model_group !== "")
      .sort((a: ModelGroup, b: ModelGroup) => a.model_group.localeCompare(b.model_group));
    return Array.from(new Map(models.map((model) => [model.model_group, model])).values());
  } catch (error) {
    console.error("Error fetching model info:", error);
    return [];
  }
};
