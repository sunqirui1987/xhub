import { describe, expect, it } from "vitest";
import {
  bodyFromForm,
  deploymentsFromInfo,
  emptyForm,
  formFromBody,
  nextCopyName,
  parseDocument,
  prettyDocument,
  formatUpdatedAt,
} from "./templateForm";

/** 验证唯一的新模板契约；全部用例只操作内存，不创建外部数据。 */
describe("路由模板文档", () => {
  /** 验证空表单输出完整骨架，并明确表示所有公开模型沿用模型管理分配。 */
  it("空模板输出完整新格式", () => {
    expect(bodyFromForm(emptyForm())).toEqual({
      ok: true,
      body: {
        routing_groups: [],
        fallbacks: [],
        context_window_fallbacks: [],
        content_policy_fallbacks: [],
        model_routes: [],
        retry_policy: { max_attempts: 1, timeout_seconds: 60, failure_threshold: 3, cooldown_seconds: 60 },
      },
    });
  });

  /** 验证模型规则只保存公开模型和策略，导入后的草稿与原文档隔离。 */
  it("模型规则原样导出导入且不保存部署权重", () => {
    const body = {
      model_routes: [{ model: "gpt-6-sol", strategy: "least-busy" }],
      retry_policy: { max_attempts: 2, timeout_seconds: 17, failure_threshold: 0, cooldown_seconds: 9 },
    };
    const form = formFromBody(body);
    expect(bodyFromForm(form)).toEqual({ ok: true, body });
    expect(parseDocument(prettyDocument(body))).toEqual({ ok: true, body });
    form.model_routes[0].strategy = "random";
    expect(body.model_routes[0].strategy).toBe("least-busy");
    expect(JSON.stringify(body)).not.toContain("deployment_id");
    expect(JSON.stringify(body)).not.toContain("weight");
  });

  /** 验证尝试次数的数值边界，失败时拒绝保存。 */
  it.each(["", "-1", "NaN", "Infinity", "1.5"])("拒绝非法尝试次数 %s", (value) => {
    expect(bodyFromForm({ ...emptyForm(), max_attempts: value }).ok).toBe(false);
  });

  /** 验证同一公开模型只能出现一次，策略必须来自注册表。 */
  it("拒绝重复模型和未知策略", () => {
    const rule = { model: "m", strategy: "random" };
    expect(bodyFromForm({ ...emptyForm(), model_routes: [rule, rule] }).ok).toBe(false);
    expect(bodyFromForm({ ...emptyForm(), model_routes: [{ model: "m", strategy: "unknown" }] }).ok).toBe(false);
  });

  /** 验证旧字段、未知字段、不完整对象、入口和部署分配都直接失败。 */
  it.each([
    "null",
    "[]",
    '{"unexpected":1}',
    '{"model_overrides":[]}',
    '{"model_routing":[]}',
    '{"model_routes":[],"retry_policy":{}}',
    '{"model_routes":[{"model":"m","strategy":"random","endpoint_id":"chat"}],"retry_policy":{"max_attempts":1,"timeout_seconds":60,"failure_threshold":3,"cooldown_seconds":0}}',
  ])("拒绝非法对象 %s", (value) => expect(parseDocument(value).ok).toBe(false));

  /** 验证模型目录保留完整上游型号并使用稳定部署 ID。 */
  it("目录保留完整上游型号并使用稳定部署 ID", () => {
    const rows = deploymentsFromInfo([
      {
        model_name: "m",
        litellm_params: { model: "custom/vendor/model", deployment_id: "canonical" },
        model_info: { id: "record", transport: "openai-chat", endpoint_types: ["chat"] },
      },
    ]);
    expect(rows[0]).toMatchObject({ model: "custom/vendor/model", deployment_id: "canonical", model_name: "m" });
    expect(deploymentsFromInfo([null, {}])).toEqual([]);
  });

  /** 验证默认策略往返、旧模板继承和数值类型边界；纯内存无需清理。 */
  it("默认策略严格校验并保留旧模板继承", () => {
    const body = bodyFromForm(emptyForm());
    expect(body.ok && parseDocument(JSON.stringify(body.body))).toMatchObject({ ok: true });
    if (!body.ok) throw new Error("新建模板应合法");
    for (const strategy of ["", "invalid", null, 1])
      expect(parseDocument(JSON.stringify({ ...body.body, routing_strategy: strategy }))).toEqual({ ok: false });
    expect(bodyFromForm({ ...emptyForm(), routing_strategy: "invalid" })).toEqual({
      ok: false,
      field: "routing_strategy",
    });
    expect(bodyFromForm(formFromBody({ model_routes: [], retry_policy: body.body.retry_policy }))).toEqual({
      ok: true,
      body: { model_routes: [], retry_policy: body.body.retry_policy },
    });
    expect(
      parseDocument(
        JSON.stringify({
          ...body.body,
          retry_policy: { ...(body.body.retry_policy as object), timeout_seconds: "60" },
        }),
      ),
    ).toEqual({ ok: false });
    expect(
      bodyFromForm({ ...emptyForm(), failure_threshold: "0", cooldown_seconds: "0", timeout_seconds: "0.5" }).ok,
    ).toBe(true);
  });

  /** 验证复制命名和日期辅助函数处理边界输入。 */
  it("辅助函数处理空值", () => {
    expect(nextCopyName("m", ["copy m", "copy m 2"], (name) => "copy " + name)).toBe("copy m 3");
    expect(formatUpdatedAt("broken")).toBe("");
  });
});

/** 验证独立权重导入导出、空列表与省略语义、边界错误；内存对象无需外部清理。 */
it("模板模型权重完整往返并拒绝无效配置", () => {
  for (const allocations of [
    undefined,
    [],
    [
      { deployment_id: "a", weight: 0 },
      { deployment_id: "b", weight: 7 },
    ],
  ]) {
    const form = {
      ...emptyForm(),
      model_routes: [{ model: "m", strategy: "traffic-split", ...(allocations === undefined ? {} : { allocations }) }],
    };
    const result = bodyFromForm(form);
    expect(result.ok).toBe(true);
    if (result.ok) expect(parseDocument(JSON.stringify(result.body))).toEqual(result);
  }
  for (const allocations of [
    null,
    "bad",
    [null],
    [{ deployment_id: "a", weight: 0 }],
    [{ deployment_id: "a", weight: -1 }],
    [{ deployment_id: "a", weight: "1" }],
    [{ deployment_id: "a", weight: 1, unknown: true }],
    [
      { deployment_id: "a", weight: 1 },
      { deployment_id: "a", weight: 2 },
    ],
  ]) {
    const doc = {
      model_routes: [{ model: "m", strategy: "traffic-split", allocations }],
      retry_policy: { max_attempts: 1, timeout_seconds: 60, failure_threshold: 0, cooldown_seconds: 0 },
    };
    expect(parseDocument(JSON.stringify(doc)).ok).toBe(false);
  }
  expect(
    bodyFromForm({ ...emptyForm(), model_routes: [{ model: "m", strategy: "least-busy", allocations: [] }] }).ok,
  ).toBe(false);
});
