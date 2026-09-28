import { describe, expect, it } from "vitest";
import { formatClosedRangeLabel } from "./advanced_date_picker";

describe("formatClosedRangeLabel", () => {
  it("does not render the zh-CN moment pattern D MMM", () => {
    const at = new Date(2026, 8, 21, 8, 14, 0);
    const label = formatClosedRangeLabel(at, at, "zh-CN");
    expect(label).not.toContain("21 9月, 08:14");
    expect(label).toBe("9月21日 08:14 - 9月21日 08:14");
  });
});
