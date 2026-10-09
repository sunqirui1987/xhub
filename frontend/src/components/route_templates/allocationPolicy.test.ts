import { describe, it, expect } from "vitest";
import { policyError } from "./allocationPolicy";
/** 相对权重允许任意总和和零排除，拒绝无效数字及重复身份；内存测试无需清理。 */
describe("相对权重校验", () => {
  it("3:7 和零排除均合法", () =>
    expect(
      policyError({
        strategy: "traffic-split",
        allocations: [
          { deployment_id: "a", weight: 3 },
          { deployment_id: "b", weight: 7 },
          { deployment_id: "c", weight: 0 },
        ],
      }),
    ).toBe(""));
  it.each([NaN, Infinity, -1])("拒绝非法数字 %s", (weight) =>
    expect(policyError({ strategy: "traffic-split", allocations: [{ deployment_id: "a", weight }] })).not.toBe(""),
  );
  it("拒绝全零与重复部署", () => {
    expect(policyError({ strategy: "traffic-split", allocations: [{ deployment_id: "a", weight: 0 }] })).not.toBe("");
    expect(
      policyError({
        strategy: "traffic-split",
        allocations: [
          { deployment_id: "a", weight: 1 },
          { deployment_id: "a", weight: 2 },
        ],
      }),
    ).not.toBe("");
  });
});
