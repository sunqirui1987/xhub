import { describe, expect, it } from "vitest";
import { priceSelectOptions } from "./priceModelOptions";

describe("priceSelectOptions", () => {
  /** 前置有效目录；验证排序、标签和去重，重复条目不覆盖首项；纯函数无需清理。 */
  it("keeps catalog labels and order without duplicate IDs", () => {
    expect(
      priceSelectOptions([{ id: "chat", label: "对话" }, { id: "chat", label: "重复" }, { id: "embedding" }], "chat"),
    ).toEqual([
      { id: "chat", label: "对话" },
      { id: "embedding", label: "embedding" },
    ]);
  });
  /** 前置空目录与旧值；验证旧值保留及空值不产生额外选项；纯函数无需清理。 */
  it("preserves an existing value outside the catalog", () => {
    expect(priceSelectOptions([], "legacy")).toEqual([{ id: "legacy", label: "legacy" }]);
    expect(priceSelectOptions([])).toEqual([]);
  });
  /** 前置非法接口数据；验证丢弃错误条目、缺失标签回退且不抛异常；纯函数无需清理。 */
  it("ignores malformed entries and invalid catalog bodies", () => {
    expect(priceSelectOptions([null, 4, {}, { id: 8 }, { id: " " }, { id: "valid", label: 3 }])).toEqual([
      { id: "valid", label: "valid" },
    ]);
    expect(priceSelectOptions(undefined, "legacy")).toEqual([{ id: "legacy", label: "legacy" }]);
  });
});
