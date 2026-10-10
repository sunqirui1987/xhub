import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { ImageRequestForm } from "./ImageRequestForm";

/** 前置本地表单；验证各字段值、扩展尺寸、数值转换及清除质量，回调无网络写入，自动卸载清理。 */
it("表单编辑返回明确字段且可删除可选质量", () => {
  const change = vi.fn();
  const view = render(
    <ImageRequestForm doc={{ prompt: "猫", size: "1024x1024", n: 1 }} disabled={false} onChange={change} />,
  );
  expect(screen.getByLabelText("图片尺寸")).toHaveValue("1024x1024");
  for (const [label, value, key, expected] of [
    ["图片提示词", "狗", "prompt", "狗"],
    ["图片尺寸", "2048x2048", "size", "2048x2048"],
    ["图片数量", "2", "n", 2],
    ["图片数量", "", "n", ""],
    ["图片质量", "high", "quality", "high"],
    ["图片质量", "", "quality", undefined],
  ] as const) {
    // 模拟调用方把上一次字段变更回写受控表单，确保清空质量由非空值触发。
    if (key === "quality" && value === "")
      view.rerender(<ImageRequestForm doc={{ quality: "high" }} disabled={false} onChange={change} />);
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
    expect(change).toHaveBeenLastCalledWith(key, expected);
  }
});

/** 前置异常类型草稿和请求中状态；验证空值降级及禁用所有控件，防止发送中修改；自动卸载无持久数据。 */
it("异常字段安全显示且请求中禁止编辑", () => {
  render(<ImageRequestForm doc={{ prompt: 1, size: null, n: "bad", quality: [] }} disabled onChange={vi.fn()} />);
  for (const label of ["图片提示词", "图片尺寸", "图片数量", "图片质量"])
    expect(screen.getByLabelText(label)).toBeDisabled();
  expect(screen.getByLabelText("图片提示词")).toHaveValue("");
});
