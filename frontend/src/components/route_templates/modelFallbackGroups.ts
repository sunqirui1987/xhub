import type { FallbackGroup } from "@/components/Settings/RouterSettings/Fallbacks/FallbackGroupConfig";
import type { ChainRow } from "./templateForm";

const MAX_FALLBACKS = 10;

const splitTargets = (targets: string): string[] =>
  targets
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter((item) => item !== "")
    .slice(0, MAX_FALLBACKS);

/**
 * One chain row becomes one group. An empty list still opens on group 1, so the
 * operator is not looking at a blank page before they can pick a primary model.
 */
export const groupsFromChains = (rows: ChainRow[]): FallbackGroup[] => {
  if (rows.length === 0) {
    return [{ id: "1", primaryModel: null, fallbackModels: [] }];
  }
  return rows.map((row, index) => ({
    id: String(index + 1),
    primaryModel: row.primary.trim() || null,
    fallbackModels: splitTargets(row.targets),
  }));
};

/** Groups without a primary are still being filled in, so they are not written. */
export const chainsFromGroups = (groups: FallbackGroup[]): ChainRow[] =>
  groups
    .filter((group) => group.primaryModel != null && group.primaryModel.trim() !== "")
    .map((group) => ({
      primary: group.primaryModel?.trim() ?? "",
      targets: group.fallbackModels
        .map((item) => item.trim())
        .filter((item) => item !== "")
        .slice(0, MAX_FALLBACKS)
        .join(", "),
    }));
