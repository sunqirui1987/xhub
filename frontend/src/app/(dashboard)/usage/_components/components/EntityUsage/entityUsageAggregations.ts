import { keyActivityLabel } from "@/components/UsagePage/keyActivityLabel";
import { BreakdownMetrics, DailyData, KeyMetricWithMetadata } from "@/components/UsagePage/types";

export type ExtendedDailyData = DailyData & {
  breakdown: BreakdownMetrics;
};

export type ModelBreakdownKey = "models" | "model_groups";

export interface ProviderSpendRow extends Record<string, unknown> {
  provider: string;
  spend: number;
  requests: number;
  successful_requests: number;
  failed_requests: number;
  tokens: number;
}

/** 按模型或模型组合并日报，返回按费用排序的前 N 项；空集合返回空数组。
 * 参数为有效日报以及可选维度和数量限制；实体用量页调用，无写入副作用。 */
export const getTopModels = (
  results: ExtendedDailyData[],
  modelBreakdownKey: ModelBreakdownKey,
  topModelsLimit: number,
) => {
  const modelSpend: { [key: string]: any } = {};
  results.forEach((day) => {
    Object.entries(day.breakdown[modelBreakdownKey] || {}).forEach(([model, metrics]) => {
      if (!modelSpend[model]) {
        modelSpend[model] = {
          spend: 0,
          requests: 0,
          successful_requests: 0,
          failed_requests: 0,
          tokens: 0,
        };
      }
      try {
        modelSpend[model].spend += metrics.metrics.spend;
      } catch (e) {
        console.error(`Error adding spend for ${model}: ${e}, got metrics: ${JSON.stringify(metrics)}`);
      }
      modelSpend[model].requests += metrics.metrics.api_requests;
      modelSpend[model].successful_requests += metrics.metrics.successful_requests;
      modelSpend[model].failed_requests += metrics.metrics.failed_requests;
      modelSpend[model].tokens += metrics.metrics.total_tokens;
    });
  });

  return Object.entries(modelSpend)
    .map(([model, metrics]) => ({
      key: model,
      ...metrics,
    }))
    .sort((a, b) => b.spend - a.spend)
    .slice(0, topModelsLimit);
};

/** 合并显式 Agent 日报，返回前 N 项；仅旧实体调用方使用。
 * 参数为有效日报以及可选维度和数量限制；实体用量页调用，无写入副作用。 */
export const getTopAgents = (results: ExtendedDailyData[], topAgentsLimit: number) => {
  const agentSpend: { [key: string]: any } = {};
  results.forEach((day) => {
    Object.entries(day.breakdown.entities || {}).forEach(([agentId, data]) => {
      if (!agentSpend[agentId]) {
        agentSpend[agentId] = {
          spend: 0,
          requests: 0,
          successful_requests: 0,
          failed_requests: 0,
          tokens: 0,
          agent_name: (data.metadata as any)?.agent_name || agentId,
        };
      }
      agentSpend[agentId].spend += data.metrics.spend;
      agentSpend[agentId].requests += data.metrics.api_requests;
      agentSpend[agentId].successful_requests += data.metrics.successful_requests;
      agentSpend[agentId].failed_requests += data.metrics.failed_requests;
      agentSpend[agentId].tokens += data.metrics.total_tokens;
    });
  });

  return Object.entries(agentSpend)
    .map(([agentId, metrics]) => ({
      key: metrics.agent_name,
      ...metrics,
    }))
    .sort((a, b) => b.spend - a.spend)
    .slice(0, topAgentsLimit);
};

/** 合并密钥费用及明确的标签元数据，返回前 N 项；缺少实体明细也可处理。
 * 参数为有效日报以及可选维度和数量限制；实体用量页调用，无写入副作用。 */
export const getTopAPIKeys = (results: ExtendedDailyData[], topKeysLimit: number) => {
  const keySpend: { [key: string]: KeyMetricWithMetadata } = {};
  results.forEach((day) => {
    Object.entries(day.breakdown.api_keys || {}).forEach(([key, metrics]) => {
      if (!keySpend[key]) {
        keySpend[key] = {
          metrics: {
            spend: 0,
            prompt_tokens: 0,
            completion_tokens: 0,
            total_tokens: 0,
            api_requests: 0,
            successful_requests: 0,
            failed_requests: 0,
            cache_read_input_tokens: 0,
            cache_creation_input_tokens: 0,
          },
          metadata: {
            key_alias: metrics.metadata.key_alias,
            team_id: metrics.metadata.team_id || null,
            user_email: metrics.metadata.user_email,
            tags: [],
          },
        };
      }
      // 只使用接口明确提供的标签，不把团队、用户或组织 ID 当成标签；跨日按标签累加。
      for (const tag of metrics.metadata.tags ?? []) {
        const existing = keySpend[key].metadata.tags!.find((item) => item.tag === tag.tag);
        if (existing) existing.usage += tag.usage;
        else keySpend[key].metadata.tags!.push({ ...tag });
      }
      keySpend[key].metrics.spend += metrics.metrics.spend;
      keySpend[key].metrics.prompt_tokens += metrics.metrics.prompt_tokens;
      keySpend[key].metrics.completion_tokens += metrics.metrics.completion_tokens;
      keySpend[key].metrics.total_tokens += metrics.metrics.total_tokens;
      keySpend[key].metrics.api_requests += metrics.metrics.api_requests;
      keySpend[key].metrics.successful_requests += metrics.metrics.successful_requests;
      keySpend[key].metrics.failed_requests += metrics.metrics.failed_requests;
      keySpend[key].metrics.cache_read_input_tokens += metrics.metrics.cache_read_input_tokens || 0;
      keySpend[key].metrics.cache_creation_input_tokens += metrics.metrics.cache_creation_input_tokens || 0;
    });
  });

  return Object.entries(keySpend)
    .map(([api_key, metrics]) => ({
      api_key,
      key_alias: keyActivityLabel(metrics.metadata),
      tags: metrics.metadata.tags || "-",
      spend: metrics.metrics.spend,
    }))
    .sort((a, b) => b.spend - a.spend)
    .slice(0, topKeysLimit);
};

/** 合并供应商费用、请求和 Token，保留零费用及失败调用，返回按费用排序的全部行。
 * 参数为有效日报以及可选维度和数量限制；实体用量页调用，无写入副作用。 */
export const getProviderSpend = (results: ExtendedDailyData[]): ProviderSpendRow[] => {
  const providerSpend: Record<string, ProviderSpendRow> = {};
  results.forEach((day) => {
    Object.entries(day.breakdown.providers || {}).forEach(([provider, metrics]) => {
      if (!providerSpend[provider]) {
        providerSpend[provider] = {
          provider,
          spend: 0,
          requests: 0,
          successful_requests: 0,
          failed_requests: 0,
          tokens: 0,
        };
      }
      try {
        providerSpend[provider].spend += metrics.metrics.spend;
        providerSpend[provider].requests += metrics.metrics.api_requests;
        providerSpend[provider].successful_requests += metrics.metrics.successful_requests;
        providerSpend[provider].failed_requests += metrics.metrics.failed_requests;
        providerSpend[provider].tokens += metrics.metrics.total_tokens;
      } catch (e) {
        console.error(`Error processing provider ${provider}: ${e}`);
      }
    });
  });

  return Object.values(providerSpend)
    .sort((a, b) => b.spend - a.spend);
};
