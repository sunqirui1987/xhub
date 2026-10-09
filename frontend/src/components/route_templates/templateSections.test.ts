import { describe, it, expect } from "vitest";
import { validateSections } from "./templateSections";
import { bodyFromForm, formFromBody, emptyForm, parseDocument } from "./templateForm";
describe("模板内组与回退契约", () => {
  /** 验证三个部分完整往返、显式禁用和草稿隔离；内存对象，无外部数据清理。 */
  it("roundtrips all sections without modifying the source", () => {
    const initial = bodyFromForm(emptyForm());
    if (!initial.ok) throw new Error(initial.field);
    const body = {
      ...initial.body,
      routing_groups: [
        {
          group_name: "g",
          models: ["a"],
          routing_strategy: "traffic-split",
          routing_strategy_args: { allocations: [{ deployment_id: "a1", weight: 3 }] },
        },
      ],
      fallbacks: [{ g: ["b"] }],
      context_window_fallbacks: [{ a: [] }],
      content_policy_fallbacks: [],
    };
    expect(parseDocument(JSON.stringify(body))).toEqual({ ok: true, body });
    const draft = formFromBody(body);
    expect(bodyFromForm(draft)).toEqual({ ok: true, body });
    draft.fallbacks![0].g.push("c");
    expect(body.fallbacks[0].g).toEqual(["b"]);
  });
  /** 验证重复成员、名称、非法权重、循环和最大目标数；失败不得进入保存，无需清理。 */
  it.each([
    { routing_groups: [{ group_name: "g", models: ["a", "a"], routing_strategy: "random" }] },
    {
      routing_groups: [
        {
          group_name: "g",
          models: ["a"],
          routing_strategy: "traffic-split",
          routing_strategy_args: { allocations: [{ deployment_id: "a1", weight: 0 }] },
        },
      ],
    },
    { fallbacks: [{ a: ["b", "b"] }] },
    { fallbacks: [{ a: ["a"] }] },
    { fallbacks: [{ a: ["b"] }], content_policy_fallbacks: [{ b: ["a"] }] },
    { fallbacks: [{ a: Array.from({ length: 33 }, (_, i) => "m" + i) }] },
    { fallbacks: null },
  ])("rejects malformed sections %j", (body) => expect(validateSections(body)).not.toBe(""));
  /** 验证未配置和最大32目标边界合法，空链表示显式禁用；内存状态自动释放。 */
  it("accepts inherited and explicitly disabled chains", () => {
    expect(validateSections({})).toBe("");
    expect(validateSections({ fallbacks: [{ a: [] }] })).toBe("");
    expect(validateSections({ fallbacks: [{ a: Array.from({ length: 32 }, (_, i) => "m" + i) }] })).toBe("");
  });
});
