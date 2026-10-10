// 模型可用列表投影：保留后端验证的端点声明，价格分类不参与协议能力推断。

import { excludeProxyWideSentinel } from "@/components/key_team_helpers/fetch_available_models_team_key";
import { modelAvailableCall, userAvailableModelsCall } from "@/components/networking";

/** ModelEndpoint 是后端公开的调用契约，包含协议、真实路径、执行传输和任务后续动作。
 * 不携带上游地址或凭据；客户端只能调用当前模型明确声明的绑定。
 */
export interface ModelEndpoint {
  endpoint_id: string;
  transport: string;
  kind: "adapted" | "bypass";
  protocol: string;
  family: string;
  method: string;
  path: string;
  actions?: { name: string; method: string; public_path: string; task_query?: string }[];
}

/** ModelGroup 表示调用方有权使用的公开模型别名及端点并集。
 * mode 只用于展示；endpoints 为空表示没有可调用绑定，禁止猜测为聊天模型。
 */
export interface ModelGroup {
  unavailable_reason?: string;
  endpoints?: ModelEndpoint[];
  model_group: string;
  mode?: string;
  supports_reasoning?: boolean;
  supported_reasoning_efforts?: string[] | null;
}

/** AvailableModel 是可用列表的网络字段投影；供应商壳不属于可调用部署。 */
interface AvailableModel {
  unavailable_reason?: string;
  endpoints?: ModelEndpoint[];
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

/** isProviderShell 判断是否为供应商壳。参数 item：网络模型记录；返回：是否应从模型选择器排除。
 * 调用：fetchAvailableModels。测试：fetch_models.test.tsx。
 */
const isProviderShell = (item: AvailableModel): boolean =>
  item.role === "provider" || item.model_info?.role === "provider";

/** toModelGroup 将一条网络模型记录转换为选择器数据。
 * 参数 item：已排除供应商壳的记录；返回：模型名、原样端点和推理能力。
 * 调用：fetchAvailableModels。测试：fetch_models.test.tsx。
 */
const toModelGroup = (item: AvailableModel): ModelGroup => {
  const groupName = (item.model_group || item.id || item.model_name) ?? "";
  return {
    model_group: groupName,
    endpoints: item.endpoints ?? [],
    ...(item.unavailable_reason && { unavailable_reason: item.unavailable_reason }),
    ...(item.mode && { mode: item.mode }),
    ...((item.supports_reasoning === true || item.capabilities?.includes("reasoning")) && { supports_reasoning: true }),
    ...(item.supported_reasoning_efforts !== undefined && {
      supported_reasoning_efforts: item.supported_reasoning_efforts,
    }),
  };
};

/** fetchAvailableModelsForTeam 读取团队授权模型名称，用于权限配置。
 * 参数 accessToken：会话令牌；teamId：团队 ID；返回：去重排序的名称列表。
 * 该列表不提供端点声明，不可直接作为 Playground 的调用能力来源。
 * 调用：团队配置组件。测试：fetch_models.test.tsx。
 */
export const fetchAvailableModelsForTeam = async (accessToken: string, teamId: string): Promise<ModelGroup[]> => {
  const response = await modelAvailableCall(accessToken, "", "", false, teamId);
  const modelNames: string[] = (response?.data ?? []).map((model: { id: string }) => model.id);

  return excludeProxyWideSentinel(Array.from(new Set(modelNames)))
    .sort((a, b) => a.localeCompare(b))
    .map((model) => ({ model_group: model }));
};

/**
 * fetchAvailableModels 读取当前调用方有权使用的模型及明确端点绑定。
 * 参数 accessToken：会话或虚拟密钥；strict：调试台需要区分读取失败时传 true；返回公开模型列表。
 * 默认读取失败返回空列表；strict 模式向调用方抛出原始异常。
 * 使用 /model/available，管理目录与价格目录不作为调用权限或能力来源。
 * 调用：ChatUI、CompareUI。测试：fetch_models.test.tsx。
 */
export const fetchAvailableModels = async (accessToken: string, strict = false): Promise<ModelGroup[]> => {
  try {
    const fetchedModels = await userAvailableModelsCall(accessToken);
    const fetchedData: unknown = fetchedModels?.data;
    const models: ModelGroup[] = (Array.isArray(fetchedData) ? fetchedData : [])
      .filter(
        (item): item is AvailableModel =>
          typeof item === "object" && item !== null && !isProviderShell(item as AvailableModel),
      )
      .map((item) => {
        const group = toModelGroup(item);
        const mode = item.category || item.mode;
        return { ...group, ...(mode && mode !== "other" ? { mode } : {}) };
      })
      .filter((model: ModelGroup) => model.model_group !== "")
      .sort((a: ModelGroup, b: ModelGroup) => a.model_group.localeCompare(b.model_group));
    return Array.from(new Map(models.map((model) => [model.model_group, model])).values());
  } catch (error) {
    if (strict) throw error;
    return [];
  }
};
