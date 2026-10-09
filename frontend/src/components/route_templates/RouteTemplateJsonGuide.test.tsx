import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import RouteTemplateJsonGuide, { ROUTE_TEMPLATE_EXAMPLE } from "./RouteTemplateJsonGuide";
import { parseDocument } from "./templateForm";

describe("RouteTemplateJsonGuide", () => {
  /** 前置当前指南组件；验证可见字段与严格模板一致，不再展示废弃字段；纯渲染由测试环境清理。 */
  it("documents only supported template fields", () => {
    render(<RouteTemplateJsonGuide />);
    expect(screen.getByText("路由模板 JSON 指南")).toBeInTheDocument();
    for (const field of [
      "routing_groups",
      "model_routes",
      "retry_policy.max_attempts",
      "retry_policy.timeout_seconds",
      "retry_policy.failure_threshold",
      "retry_policy.cooldown_seconds",
    ])
      expect(screen.getByRole("cell", { name: field, exact: true })).toBeInTheDocument();
    for (const field of ["model_routing", "model_overrides", "num_retries", "endpoint_id", "allocations"])
      expect(screen.queryByText(field, { exact: true })).not.toBeInTheDocument();
  });
  /** 前置可导入示例；验证正常解析、负数和未知字段拒绝；纯函数测试不产生外部数据。 */
  it("provides a valid example and rejects invalid edits", () => {
    expect(parseDocument(JSON.stringify(ROUTE_TEMPLATE_EXAMPLE))).toMatchObject({ ok: true });
    expect(parseDocument(JSON.stringify({ ...ROUTE_TEMPLATE_EXAMPLE, num_retries: 1 }))).toEqual({ ok: false });
    expect(parseDocument(JSON.stringify({ ...ROUTE_TEMPLATE_EXAMPLE, stream_timeout: 120 }))).toEqual({ ok: false });
  });
  /** 前置指南示例分区及可用剪贴板；验证复制完整正文和成功提示，卸载清理 UI 并恢复剪贴板方法。 */
  it("copies the complete example with success feedback", async () => {
    const user = userEvent.setup();
    const write = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue();
    try {
      render(<RouteTemplateJsonGuide />);
      await user.click(screen.getByRole("tab", { name: "可导入示例" }));
      const example = screen.getByRole("region", { name: "完整模板示例" });
      await user.click(within(example).getByRole("button", { name: "复制 JSON" }));
      expect(write).toHaveBeenCalledWith(JSON.stringify(ROUTE_TEMPLATE_EXAMPLE, null, 2));
      expect(within(example).getByRole("status")).toHaveTextContent("已复制");
    } finally {
      write.mockRestore();
    }
  });
  /** 前置剪贴板权限拒绝；验证复制错误可见且示例保留，卸载清理 UI 并恢复方法，无外部数据。 */
  it("reports clipboard rejection without hiding the example", async () => {
    const user = userEvent.setup();
    const write = vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    try {
      render(<RouteTemplateJsonGuide />);
      await user.click(screen.getByRole("tab", { name: "可导入示例" }));
      const example = screen.getByRole("region", { name: "完整模板示例" });
      await user.click(within(example).getByRole("button", { name: "复制 JSON" }));
      expect(within(example).getByRole("status")).toHaveTextContent("复制失败，请手动选择代码复制");
      expect(example).toHaveTextContent('"routing_strategy": "cost-based-routing"');
    } finally {
      write.mockRestore();
    }
  });
});
