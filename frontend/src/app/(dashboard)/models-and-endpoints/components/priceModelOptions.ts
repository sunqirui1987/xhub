/** 价格分类下拉选项；id 为保存值，label 为目录展示名称。 */
export type PriceSelectOption = { id: string; label: string };

/** 构建价格表单选项；参数为可能不完整的目录与当前值，返回去重选项，供三种分类下拉调用。
 * 非数组或无效条目被忽略，缺失标签回退 ID；当前旧值不在目录时追加，避免编辑造成数据丢失。无副作用。 */
export function priceSelectOptions(entries: unknown, current = ""): PriceSelectOption[] {
  const options = new Map<string, PriceSelectOption>();
  for (const entry of Array.isArray(entries) ? entries : []) {
    if (!entry || typeof entry !== "object" || typeof entry.id !== "string" || !entry.id.trim()) continue;
    if (!options.has(entry.id)) {
      options.set(entry.id, {
        id: entry.id,
        label: typeof entry.label === "string" && entry.label.trim() ? entry.label : entry.id,
      });
    }
  }
  if (current && !options.has(current)) options.set(current, { id: current, label: current });
  return [...options.values()];
}
