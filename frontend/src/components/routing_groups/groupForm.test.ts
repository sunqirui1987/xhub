import { describe, it, expect } from "vitest";
import { groupFormError, reconcileGroup, type GroupConfig, type CatalogModel } from "./groupForm";
const catalog: CatalogModel[] = [
  { model_name: "a", deployments: [{ id: "a1" }, { id: "a2" }] },
  { model_name: "b", deployments: [{ id: "b1" }] },
];
const draft: GroupConfig = { group_name: "group", models: ["a", "b"], routing_strategy: "traffic-split" };
describe("路由组表单", () => {
  /** 前置完整目录，验证合法与64字符边界、名称冲突、占用及未知成员；纯函数无需清理。 */
  it("校验名称成员与策略", () => {
    expect(groupFormError(draft, [], catalog)).toBe("");
    expect(groupFormError({ ...draft, group_name: "字".repeat(64) }, [], catalog)).toBe("");
    for (const group_name of ["", "default", "a", "x y", "x*", "字".repeat(65)])
      expect(groupFormError({ ...draft, group_name }, [], catalog)).not.toBe("");
    for (const models of [[], ["a", "a"], ["missing"]])
      expect(groupFormError({ ...draft, models }, [], catalog)).not.toBe("");
    expect(groupFormError(draft, [{ ...draft, group_name: "owner" }], catalog)).toContain("其他路由组");
    expect(groupFormError(draft, [draft], catalog, true)).toBe("");
    expect(groupFormError({ ...draft, routing_strategy: "unsupported" }, [], catalog)).not.toBe("");
  });
  /** 验证权重非法数、零和重复引用，以及成员变更保留与清除权重；内存输入无副作用和清理。 */
  it("校验并协调部署权重", () => {
    expect(reconcileGroup(draft, catalog).routing_strategy_args?.allocations).toEqual([
      { deployment_id: "a1", weight: 1 },
      { deployment_id: "a2", weight: 1 },
      { deployment_id: "b1", weight: 1 },
    ]);
    const weighted = {
      ...draft,
      routing_strategy_args: {
        allocations: [
          { deployment_id: "a1", weight: 3 },
          { deployment_id: "b1", weight: 7 },
        ],
      },
    };
    expect(groupFormError(weighted, [], catalog)).toBe("");
    expect(reconcileGroup({ ...weighted, models: ["a"] }, catalog).routing_strategy_args?.allocations).toEqual([
      { deployment_id: "a1", weight: 3 },
      { deployment_id: "a2", weight: 1 },
    ]);
    expect(
      reconcileGroup({ ...weighted, routing_strategy: "least-busy" }, catalog).routing_strategy_args,
    ).toBeUndefined();
    for (const weight of [-1, NaN, Infinity, 0])
      expect(
        groupFormError(
          { ...draft, routing_strategy_args: { allocations: [{ deployment_id: "a1", weight }] } },
          [],
          catalog,
        ),
      ).not.toBe("");
    expect(
      groupFormError(
        { ...draft, routing_strategy_args: { allocations: [{ deployment_id: "missing", weight: 1 }] } },
        [],
        catalog,
      ),
    ).not.toBe("");
    expect(
      groupFormError(
        {
          ...draft,
          routing_strategy_args: {
            allocations: [
              { deployment_id: "a1", weight: 1 },
              { deployment_id: "a1", weight: 2 },
            ],
          },
        },
        [],
        catalog,
      ),
    ).not.toBe("");
    expect(groupFormError({ ...weighted, routing_strategy: "simple-shuffle" }, [], catalog)).not.toBe("");
  });
});
