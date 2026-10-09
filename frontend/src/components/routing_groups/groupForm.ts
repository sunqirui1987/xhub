import type { Allocation } from "@/components/route_templates/allocationPolicy";
export type GroupConfig = {
  group_name: string;
  models: string[];
  routing_strategy: string;
  routing_strategy_args?: { allocations: Allocation[] };
};
export type CatalogModel = {
  model_name: string;
  deployments: { id: string; litellm_params?: { model?: string } }[];
  fallback_policy?: Record<string, string[]>;
};
export const groupStrategies: Record<string, string> = {
  "simple-shuffle": "简单随机",
  random: "简单随机（兼容值）",
  "least-busy": "最少并发",
  "cost-based-routing": "最低成本",
  "latency-based-routing": "最低延迟",
  "usage-based-routing": "最低用量",
  "traffic-split": "按权重分流",
};
export const groupHints: Record<string, string> = {
  "simple-shuffle": "兼容部署均匀随机选择；部署越多的成员获得的总流量越多。",
  "least-busy": "优先选择当前正在处理请求最少的部署。",
  "cost-based-routing": "比较实际费率的输入 token 单价；缺少价格的部署排在最后。",
  "latency-based-routing": "优先选择观测延迟最低的部署；跨实例统计依赖 Redis。",
  "usage-based-routing": "优先选择统计 token 用量最低的部署；跨实例统计依赖 Redis。",
  "traffic-split": "按部署相对权重随机分配；零权重不参与，未设置的部署默认权重为 1。",
};
/** 校验弹窗草稿的名称、成员占用与真实部署权重；返回中文错误或空串，不修改输入。
 * 保存前调用，后台再次验证；编辑时允许保留当前名称和原有成员。 */
export function groupFormError(
  draft: GroupConfig,
  groups: GroupConfig[],
  catalog: CatalogModel[],
  editing = false,
): string {
  if (
    !draft.group_name ||
    draft.group_name.trim() !== draft.group_name ||
    /[\s*]/.test(draft.group_name) ||
    [...draft.group_name].length > 64 ||
    draft.group_name === "default"
  )
    return "组名须为 1–64 个字符，不能含空白、星号或使用 default。";
  if (
    catalog.some((row) => row.model_name === draft.group_name) ||
    (!editing && groups.some((g) => g.group_name === draft.group_name))
  )
    return "组名已存在或与公开模型重名。";
  if (!draft.models.length || new Set(draft.models).size !== draft.models.length)
    return "请选择至少一个不重复的成员模型。";
  for (const name of draft.models) {
    if (!catalog.some((row) => row.model_name === name) || name.includes("*"))
      return "成员必须是目录中的确切公开模型。";
    if (groups.some((g) => g.group_name !== (editing ? draft.group_name : "") && g.models.includes(name)))
      return "成员模型已被其他路由组使用。";
  }
  if (!Object.hasOwn(groupStrategies, draft.routing_strategy)) return "请选择有效的路由策略。";
  const args = draft.routing_strategy_args;
  if (args && draft.routing_strategy !== "traffic-split") return "只有按权重分流可以设置部署权重。";
  if (args) {
    const validIDs = new Set(
      catalog.filter((r) => draft.models.includes(r.model_name)).flatMap((r) => r.deployments.map((d) => d.id)),
    );
    const rows = args.allocations;
    if (
      new Set(rows.map((r) => r.deployment_id)).size !== rows.length ||
      rows.some((r) => !validIDs.has(r.deployment_id) || !Number.isFinite(r.weight) || r.weight < 0)
    )
      return "权重必须非负、有限，且引用组内唯一部署。";
    if (rows.length && (!rows.some((r) => r.weight > 0) || !Number.isFinite(rows.reduce((n, r) => n + r.weight, 0))))
      return "至少一个部署的权重必须大于 0，权重总和必须有限。";
  }
  return "";
}
/** 成员变更时丢弃已移除部署的权重；参数为草稿和完整目录，返回可保存副本，无服务器写入。 */
export function reconcileGroup(draft: GroupConfig, catalog: CatalogModel[]): GroupConfig {
  if (draft.routing_strategy !== "traffic-split") {
    const { routing_strategy_args: _, ...base } = draft;
    return base;
  }
  const previous = new Map(draft.routing_strategy_args?.allocations.map((r) => [r.deployment_id, r.weight]));
  return {
    ...draft,
    routing_strategy_args: {
      allocations: catalog
        .filter((r) => draft.models.includes(r.model_name))
        .flatMap((r) => r.deployments.map((d) => ({ deployment_id: d.id, weight: previous.get(d.id) ?? 1 }))),
    },
  };
}
