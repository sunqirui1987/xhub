import { groupStrategies, type GroupConfig } from "@/components/routing_groups/groupForm";
export const fallbackFields = ["fallbacks", "context_window_fallbacks", "content_policy_fallbacks"] as const;
export type FallbackField = (typeof fallbackFields)[number];
export type TemplateSections = { routing_groups?: GroupConfig[] } & Partial<
  Record<FallbackField, Record<string, string[]>[]>
>;
/** validateSections 校验模板组和三类回退的完整结构；导入及保存共用，返回字段错误或空串。
 * 不依赖目录、不修改输入；实际模型存在性由弹窗和后台保存再次验证。 */
export function validateSections(body: Record<string, unknown>): string {
  /** record 识别JSON对象；参数为未知输入，返回类型判断，排除null和数组，无副作用。 */
  const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
  const claimed = new Set<string>(),
    names = new Set<string>();
  if (body.routing_groups !== undefined) {
    if (!Array.isArray(body.routing_groups)) return "routing_groups";
    for (const g of body.routing_groups) {
      if (
        !record(g) ||
        Object.keys(g).some(
          (k) => !["group_name", "models", "routing_strategy", "routing_strategy_args"].includes(k),
        ) ||
        typeof g.group_name !== "string" ||
        !g.group_name ||
        /\s|\*/.test(g.group_name) ||
        [...g.group_name].length > 64 ||
        g.group_name === "default" ||
        names.has(g.group_name) ||
        typeof g.routing_strategy !== "string" ||
        !Object.hasOwn({ ...groupStrategies, random: "随机" }, g.routing_strategy) ||
        !Array.isArray(g.models) ||
        !g.models.length
      )
        return "routing_groups";
      names.add(g.group_name);
      for (const name of g.models) {
        if (typeof name !== "string" || !name.trim() || name !== name.trim() || name.includes("*") || claimed.has(name))
          return "routing_groups.models";
        claimed.add(name);
      }
      if (g.routing_strategy_args !== undefined) {
        const args = g.routing_strategy_args;
        if (
          g.routing_strategy !== "traffic-split" ||
          !record(args) ||
          Object.keys(args).some((k) => k !== "allocations") ||
          !Array.isArray(args.allocations)
        )
          return "routing_strategy_args";
        const ids = new Set<string>();
        let total = 0;
        for (const row of args.allocations) {
          if (
            !record(row) ||
            Object.keys(row).some((k) => !["deployment_id", "weight"].includes(k)) ||
            typeof row.deployment_id !== "string" ||
            !row.deployment_id.trim() ||
            ids.has(row.deployment_id) ||
            typeof row.weight !== "number" ||
            !Number.isFinite(row.weight) ||
            row.weight < 0
          )
            return "routing_strategy_args.allocations";
          ids.add(row.deployment_id);
          total += row.weight;
        }
        if (args.allocations.length && (!(total > 0) || !Number.isFinite(total)))
          return "routing_strategy_args.allocations";
      }
    }
  }
  const graph = new Map<string, string[]>();
  for (const field of fallbackFields) {
    if (body[field] === undefined) continue;
    if (!Array.isArray(body[field])) return field;
    const seen = new Set<string>();
    for (const row of body[field]) {
      if (!record(row) || Object.keys(row).length !== 1) return field;
      const [name, targets] = Object.entries(row)[0];
      if (
        !name.trim() ||
        name !== name.trim() ||
        seen.has(name) ||
        !Array.isArray(targets) ||
        targets.length > 32 ||
        new Set(targets).size !== targets.length ||
        targets.some((t) => typeof t !== "string" || !t.trim() || t !== t.trim())
      )
        return field;
      seen.add(name);
      graph.set(name, [...(graph.get(name) ?? []), ...targets]);
    }
  }
  const state = new Map<string, number>();
  /** visit 深度优先检查跨类别回退循环；参数为节点，返回是否有环，仅修改本次校验状态。 */
  const visit = (name: string): boolean => {
    if (state.get(name) === 1) return true;
    if (state.get(name) === 2) return false;
    state.set(name, 1);
    if ((graph.get(name) ?? []).some(visit)) return true;
    state.set(name, 2);
    return false;
  };
  return [...graph.keys()].some(visit) ? "fallbacks.cycle" : "";
}
