import { useState } from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { TemplateGroups, TemplateFallbacks } from "./TemplateRoutingSections";
import { emptyForm, bodyFromForm, type SplitDeployment } from "./templateForm";
const deployments: SplitDeployment[] = [
  { model_name: "a", model: "up-a", deployment_id: "a1" },
  { model_name: "b", model: "up-b", deployment_id: "b1" },
];
/** Harness 模拟模板草稿所有者；输出同一正文供断言，只在内存更新，卸载清理。 */
function Harness() {
  const [form, setForm] = useState(emptyForm());
  return (
    <>
      <TemplateGroups form={form} deployments={deployments} onChange={setForm} />
      <TemplateFallbacks form={form} deployments={deployments} onChange={setForm} />
      <output data-testid="document">{JSON.stringify(bodyFromForm(form))}</output>
    </>
  );
}
describe("模板组与回退表单", () => {
  /** 验证组权重、占用保护、有序回退与显式禁用进入同一草稿；无独立后台写入，自动卸载清理。 */
  it("edits group and fallback into a single draft", () => {
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: "创建路由组" }));
    let dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("button", { name: "保存到模板" })).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("路由组名称"), { target: { value: "g" } });
    fireEvent.click(within(dialog).getByLabelText("成员模型 a"));
    fireEvent.change(within(dialog).getByLabelText("路由组策略"), { target: { value: "traffic-split" } });
    fireEvent.change(within(dialog).getByLabelText("路由组部署 a1 权重"), { target: { value: "0" } });
    expect(within(dialog).getByRole("button", { name: "保存到模板" })).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("路由组部署 a1 权重"), { target: { value: "3" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存到模板" }));
    expect(screen.getByTestId("document")).toHaveTextContent('"group_name":"g"');
    fireEvent.click(screen.getByRole("button", { name: "创建路由组" }));
    expect(screen.getByLabelText("成员模型 a")).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "取消" }));
    fireEvent.click(screen.getByRole("button", { name: "添加故障转移" }));
    dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("故障转移主模型"), { target: { value: "g" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "添加回退目标" }));
    fireEvent.change(within(dialog).getByLabelText("回退目标 1"), { target: { value: "b" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "保存到模板" }));
    expect(screen.getByTestId("document")).toHaveTextContent('"fallbacks":[{"g":["b"]}]');
    expect(screen.getByRole("button", { name: "删除路由组 g" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "编辑故障转移 g 通用错误" }));
    dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "清空回退" }));
    fireEvent.click(within(dialog).getByRole("button", { name: "保存到模板" }));
    expect(screen.getByTestId("document")).toHaveTextContent('"fallbacks":[{"g":[]}]');
  });
});
