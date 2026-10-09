import { useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import TemplateEditor from "./TemplateEditor";
import { emptyForm } from "./templateForm";

/** Harness 持有编辑器草稿；无参数，返回完整模板编辑器，内存状态由测试卸载清理。 */
function Harness() {
  const [form, setForm] = useState(emptyForm());
  return (
    <TemplateEditor
      name="fixture"
      form={form}
      deployments={[
        { deployment_id: "g1", model_name: "g", model: "upstream/g", api_base: "" },
        { deployment_id: "b1", model_name: "b", model: "upstream/b", api_base: "" },
      ]}
      accessToken={null}
      onName={() => {}}
      onChange={setForm}
      onJsonValid={() => {}}
    />
  );
}

describe("模板JSON与业务分区同步", () => {
  /** 导入合法JSON后修改故障转移，验证回到JSON时显示新草稿；失败输入须禁用表单切换，无外部数据。 */
  it("updates imported JSON after a fallback form change", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("tab", { name: "JSON" }));
    const text = screen.getByRole("textbox", { name: "JSON" });
    const body = JSON.parse((text as HTMLTextAreaElement).value);
    body.fallbacks = [{ g: ["b"] }];
    fireEvent.change(text, { target: { value: JSON.stringify(body) } });
    fireEvent.click(screen.getByRole("tab", { name: "故障转移" }));
    fireEvent.click(screen.getByRole("button", { name: "编辑故障转移 g 通用错误" }));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "清空回退" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "保存到模板" }));
    fireEvent.click(screen.getByRole("tab", { name: "JSON" }));
    const updated = screen.getByRole("textbox", { name: "JSON" });
    expect(JSON.parse((updated as HTMLTextAreaElement).value).fallbacks).toEqual([{ g: [] }]);
    fireEvent.change(updated, { target: { value: "{" } });
    expect(screen.getByRole("tab", { name: "故障转移" })).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("alert")).toBeVisible();
  });
});
