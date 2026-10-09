/** 客户覆盖策略和全局相对权重使用同一部署身份；部署 ID 是唯一选路标识。 */
export type Allocation = { deployment_id: string; weight: number };
export type AllocationPolicy = { strategy: string; allocations?: Allocation[] };
export const strategyLabels: Record<string, string> = {
  "simple-shuffle": "简单随机（推荐）",
  random: "简单随机",
  "traffic-split": "按流量分流",
  "least-busy": "最少并发",
  "cost-based-routing": "最低成本",
  "latency-based-routing": "最低延迟",
  "usage-based-routing": "最低用量",
};
/** 校验编辑或导入的相对权重；允许任意总和，拒绝重复部署及全部零，无副作用。 */
export function policyError(policy: AllocationPolicy): string {
  if (!policy || !Object.hasOwn(strategyLabels, policy.strategy)) return "请选择有效的分配方式";
  if (policy.allocations !== undefined && !Array.isArray(policy.allocations)) return "部署权重必须为列表";
  if (policy.strategy !== "traffic-split") return policy.allocations?.length ? "只有流量分流可以设置权重" : "";
  const rows = policy.allocations ?? [];
  if (!rows.length) return "请设置部署权重";
  const ids = new Set<string>();
  for (const row of rows) {
    if (!row || typeof row.deployment_id !== "string" || !row.deployment_id.trim() || ids.has(row.deployment_id))
      return "部署 ID 必须唯一且非空";
    ids.add(row.deployment_id);
    if (Object.keys(row).some((key) => !["deployment_id", "weight"].includes(key))) return "部署权重包含未知字段";
    if (typeof row.weight !== "number" || !Number.isFinite(row.weight) || row.weight < 0)
      return "权重必须为大于或等于 0 的数值";
  }
  if (!Number.isFinite(rows.reduce((total, row) => total + row.weight, 0))) return "权重总和超出数值范围";
  return rows.some((row) => row.weight > 0) ? "" : "至少一个部署的权重必须大于 0";
}
