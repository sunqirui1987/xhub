import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { setActiveLocale, I18nProvider, useI18n } from "@/i18n";
import { ImageRequestForm } from "./ImageRequestForm";

beforeEach(() => setActiveLocale("zh-CN"));

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

/** 前置中文界面和用户中文草稿；切换英文后控件及帮助说明全部翻译，草稿原样保留；自动卸载，无网络数据。 */
it("图片表单实时切换语言且保留用户输入", () => {
  /** 测试语言切换入口；无参数，返回英文切换按钮，副作用仅更新隔离 Provider。 */
  function Switch() {
    const { setLocale } = useI18n();
    return <button onClick={() => setLocale("en")}>EN</button>;
  }
  render(
    <I18nProvider initialLocale="zh-CN">
      <Switch />
      <ImageRequestForm doc={{ prompt: "用户的猫", size: "1024x1024", n: 1 }} disabled={false} onChange={vi.fn()} />
    </I18nProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "EN" }));
  expect(screen.getByLabelText("Image prompt")).toHaveValue("用户的猫");
  expect(screen.getByLabelText("Image size")).toHaveValue("1024x1024");
  expect(screen.getByPlaceholderText("Leave blank to use the provider default")).toBeVisible();
  expect(screen.getByText(/The default is one/)).toBeVisible();
  expect(screen.queryByText("图片提示词")).not.toBeInTheDocument();
});
