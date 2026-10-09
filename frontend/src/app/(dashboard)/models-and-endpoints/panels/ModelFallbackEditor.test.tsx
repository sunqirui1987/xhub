import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import ModelFallbackEditor from "./ModelFallbackEditor";
const mocks = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }));
vi.mock("@/components/networking", () => ({ apiClient: mocks }));
/** 每例重置接口实现和调用记录；提供独立目录及成功写入，避免失败模拟泄漏到后续测试，RTL负责卸载。 */
beforeEach(() => {
  vi.resetAllMocks();
  mocks.get.mockResolvedValue({
    data: [{ model_name: "a" }, { model_name: "b" }, { model_name: "c" }],
    total_pages: 1,
  });
  mocks.put.mockResolvedValue({});
});

/** 前置两页包含重复名称和自身的目录；验证读取所有分页、去重排序和三类选项共享，RTL卸载清理。 */
it("目录跨分页去重排序且排除当前模型", async () => {
  mocks.get
    .mockResolvedValueOnce({ data: [{ model_name: "z" }, { model_name: "a" }], total_pages: 2 })
    .mockResolvedValueOnce({ data: [{ model_name: "b" }, { model_name: "z" }], total_pages: 2 });
  const user = userEvent.setup();
  render(<ModelFallbackEditor model="a" accessToken="admin" onSaved={vi.fn()} initiallyOpen />);
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  expect(mocks.get.mock.calls.map((call) => call[1].query)).toEqual([
    { page: 1, size: 200 },
    { page: 2, size: 200 },
  ]);
  for (const category of ["通用错误", "上下文超限", "内容策略错误"]) {
    await user.click(screen.getByRole("button", { name: "添加" + category + "回退" }));
    const select = screen.getByLabelText(category + "目标 1");
    expect(
      within(select)
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["选择回退模型", "b", "z"]);
  }
});

/** 前置首分页成功、下一页失败；验证不允许用部分目录保存，错误可见且取消后重试能恢复，RTL清理。 */
it("后续分页失败禁止保存，重新打开可恢复", async () => {
  mocks.get
    .mockResolvedValueOnce({ data: [{ model_name: "b" }], total_pages: 2 })
    .mockRejectedValueOnce(new Error("page 2 offline"));
  const user = userEvent.setup();
  render(<ModelFallbackEditor model="a" policy={{ fallbacks: ["b"] }} accessToken="admin" onSaved={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "配置回退" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("加载失败");
  expect(screen.getByRole("button", { name: "保存回退" })).toBeDisabled();
  expect(mocks.put).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("button", { name: "配置回退" }));
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "保存回退" })).toBeEnabled();
});

/** 前置空草稿；验证空目标阻止保存、类别内重复不可选、类别间可复用及排序端点禁用，RTL自动清理。 */
it("空目标和重复选择受到限制，各错误类别独立", async () => {
  const user = userEvent.setup();
  render(<ModelFallbackEditor model="a" accessToken="admin" onSaved={vi.fn()} initiallyOpen />);
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "添加通用错误回退" }));
  expect(screen.getByRole("button", { name: "保存回退" })).toBeDisabled();
  await user.selectOptions(screen.getByLabelText("通用错误目标 1"), "b");
  expect(screen.getByRole("button", { name: "通用错误上移 1" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "通用错误下移 1" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "添加通用错误回退" }));
  expect(within(screen.getByLabelText("通用错误目标 2")).getByRole("option", { name: "b" })).toBeDisabled();
  await user.selectOptions(screen.getByLabelText("通用错误目标 2"), "c");
  await user.click(screen.getByRole("button", { name: "添加内容策略错误回退" }));
  expect(within(screen.getByLabelText("内容策略错误目标 1")).getByRole("option", { name: "b" })).toBeEnabled();
  await user.selectOptions(screen.getByLabelText("内容策略错误目标 1"), "b");
  expect(screen.getByRole("button", { name: "保存回退" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "通用错误删除 1" }));
  expect(screen.getByLabelText("通用错误目标 1")).toHaveValue("c");
  expect(screen.queryByLabelText("通用错误目标 2")).not.toBeInTheDocument();
});

/** 前置每类32个目标；验证各类别分别限制、删除后可添加且空行不能保存，RTL清理，无持久数据。 */
it("每类最多32个目标，删除后恢复添加", async () => {
  const names = Array.from({ length: 32 }, (_, i) => "target-" + i);
  mocks.get.mockResolvedValue({ data: names.map((model_name) => ({ model_name })), total_pages: 1 });
  const user = userEvent.setup();
  render(
    <ModelFallbackEditor
      model="a"
      policy={{ fallbacks: names, context_window_fallbacks: names, content_policy_fallbacks: names }}
      accessToken="admin"
      onSaved={vi.fn()}
      initiallyOpen
    />,
  );
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  for (const category of ["通用错误", "上下文超限", "内容策略错误"]) {
    expect(screen.getByRole("button", { name: "添加" + category + "回退" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: category + "删除 32" }));
    expect(screen.getByRole("button", { name: "添加" + category + "回退" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "添加" + category + "回退" }));
    expect(screen.getByLabelText(category + "目标 32")).toHaveValue("");
    expect(screen.getByRole("button", { name: "保存回退" })).toBeDisabled();
  }
});

/** 前置仅自身目录和失效历史目标；验证禁止新增但可删除失效项后清空保存，RTL卸载，无外部资源。 */
it("没有其他模型时仍可移除不可用历史目标", async () => {
  mocks.get.mockResolvedValue({ data: [{ model_name: "a" }], total_pages: 1 });
  const user = userEvent.setup();
  render(
    <ModelFallbackEditor
      model="a"
      policy={{ fallbacks: ["removed"] }}
      accessToken="admin"
      onSaved={vi.fn()}
      initiallyOpen
    />,
  );
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  expect(screen.getByRole("option", { name: "removed（不可用）" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "添加通用错误回退" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "通用错误删除 1" }));
  await user.click(screen.getByRole("button", { name: "保存回退" }));
  expect(mocks.put).toHaveBeenCalledWith("/model/fallback", {
    accessToken: "admin",
    body: { model_name: "a", policy: { fallbacks: [], context_window_fallbacks: [], content_policy_fallbacks: [] } },
  });
});
/** 前置独立内存目录；验证打开、排序、三类完整保存与刷新，无服务调用，RTL卸载清理。 */
it("每类回退按草稿顺序保存，排除自身", async () => {
  const saved = vi.fn();
  const user = userEvent.setup();
  render(<ModelFallbackEditor model="a" policy={{ fallbacks: ["b", "c"] }} accessToken="admin" onSaved={saved} />);
  await user.click(screen.getByRole("button", { name: "配置回退" }));
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  expect(screen.getByLabelText("通用错误目标 1")).not.toContainHTML('value="a"');
  await user.click(screen.getByRole("button", { name: "通用错误下移 1" }));
  await user.click(screen.getByRole("button", { name: "添加上下文超限回退" }));
  await user.selectOptions(screen.getByLabelText("上下文超限目标 1"), "b");
  await user.click(screen.getByRole("button", { name: "保存回退" }));
  expect(mocks.put).toHaveBeenCalledWith("/model/fallback", {
    accessToken: "admin",
    body: {
      model_name: "a",
      policy: { fallbacks: ["c", "b"], context_window_fallbacks: ["b"], content_policy_fallbacks: [] },
    },
  });
  expect(saved).toHaveBeenCalledOnce();
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});
/** 前置保存错误；验证草稿不丢失、清空显式保存和取消无写入，RTL自动清理。 */
it("保存失败保留草稿，清空后可以重新保存", async () => {
  const user = userEvent.setup();
  mocks.put.mockRejectedValueOnce(new Error("fallback cycle"));
  render(<ModelFallbackEditor model="a" policy={{ fallbacks: ["b"] }} accessToken="admin" onSaved={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "配置回退" }));
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "保存回退" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("fallback cycle");
  expect(screen.getByLabelText("通用错误目标 1")).toHaveValue("b");
  await user.click(screen.getByRole("button", { name: "清空回退" }));
  await user.click(screen.getByRole("button", { name: "保存回退" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByRole("button", { name: "配置回退" })).toBeVisible();
  expect(mocks.put).toHaveBeenLastCalledWith("/model/fallback", {
    accessToken: "admin",
    body: { model_name: "a", policy: { fallbacks: [], context_window_fallbacks: [], content_policy_fallbacks: [] } },
  });
  await user.click(screen.getByRole("button", { name: "配置回退" }));
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(mocks.put).toHaveBeenCalledTimes(2);
});
/** 前置目录读取失败；验证可观察错误与禁止保存，内存模拟和RTL自动清理。 */
it("目录失败时禁止写入", async () => {
  mocks.get.mockRejectedValue(new Error("offline"));
  const user = userEvent.setup();
  render(<ModelFallbackEditor model="a" accessToken="admin" onSaved={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "配置回退" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("加载失败");
  expect(screen.getByRole("button", { name: "保存回退" })).toBeDisabled();
});

/** 前置已保存策略和卡片容器；验证 Portal 弹窗、取消/Escape/X不写入、焦点恢复及重开草稿重置；RTL卸载清理。 */
it("配置回退在独立弹窗打开，关闭丢弃修改并恢复入口焦点", async () => {
  const user = userEvent.setup();
  const { container } = render(
    <ModelFallbackEditor model="a" policy={{ fallbacks: ["b"] }} accessToken="admin" onSaved={vi.fn()} />,
  );
  const trigger = screen.getByRole("button", { name: "配置回退" });
  for (const action of ["cancel", "escape", "close"]) {
    await user.click(trigger);
    const dialog = await screen.findByRole("dialog", { name: "a 回退配置" });
    expect(container).not.toContainElement(dialog);
    await waitFor(() => expect(within(dialog).queryByRole("status")).not.toBeInTheDocument());
    expect(within(dialog).getByLabelText("通用错误目标 1")).toHaveValue("b");
    await user.click(within(dialog).getByRole("button", { name: "清空回退" }));
    if (action === "cancel") await user.click(within(dialog).getByRole("button", { name: "取消" }));
    else if (action === "escape") await user.keyboard("{Escape}");
    else await user.click(within(dialog).getByRole("button", { name: /^(Close|关闭)$/ }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  }
  expect(mocks.put).not.toHaveBeenCalled();
});
/** 前置待完成写入；验证保存中禁止关闭和编辑，成功后弹窗关闭并刷新；手动完成Promise，RTL卸载清理。 */
it("保存期间保留弹窗，完成后关闭", async () => {
  const user = userEvent.setup();
  const saved = vi.fn();
  let finish!: () => void;
  mocks.put.mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
  );
  render(<ModelFallbackEditor model="a" accessToken="admin" onSaved={saved} />);
  await user.click(screen.getByRole("button", { name: "配置回退" }));
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  await user.click(screen.getByRole("button", { name: "保存回退" }));
  expect(screen.getByRole("button", { name: "正在保存…" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
  expect(screen.queryByRole("button", { name: /^(Close|关闭)$/ })).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog", { name: "a 回退配置" })).toBeVisible();
  await act(async () => finish());
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(saved).toHaveBeenCalledOnce();
});
/** 前置调用方已有弹窗并指定initiallyOpen；验证直接加载内嵌表单，无额外弹窗/入口；RTL卸载清理。 */
it("已有弹窗可复用内嵌表单", async () => {
  render(
    <ModelFallbackEditor model="a" policy={{ fallbacks: ["b"] }} accessToken="admin" onSaved={vi.fn()} initiallyOpen />,
  );
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  expect(screen.getByLabelText("通用错误目标 1")).toHaveValue("b");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "配置回退" })).not.toBeInTheDocument();
});

/** 前置弹窗直接打开及延迟保存；验证首次无写入、忙碌通知、失败保留和取消回调，RTL卸载清理。 */
it("弹窗直接打开并在保存期间报告忙碌状态", async () => {
  const user = userEvent.setup();
  const busy = vi.fn(),
    cancel = vi.fn();
  let reject!: (error: Error) => void;
  mocks.put.mockImplementationOnce(
    () =>
      new Promise((_, no) => {
        reject = no;
      }),
  );
  render(
    <ModelFallbackEditor
      model="a"
      policy={{ fallbacks: ["b"] }}
      accessToken="admin"
      onSaved={vi.fn()}
      initiallyOpen
      onBusyChange={busy}
      onCancel={cancel}
    />,
  );
  await screen.findByLabelText("通用错误目标 1");
  await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
  expect(mocks.put).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "保存回退" }));
  expect(busy).toHaveBeenLastCalledWith(true);
  expect(screen.getByRole("button", { name: "取消" })).toBeDisabled();
  reject(new Error("写入失败"));
  expect(await screen.findByRole("alert")).toHaveTextContent("写入失败");
  expect(busy).toHaveBeenLastCalledWith(false);
  expect(screen.getByLabelText("通用错误目标 1")).toHaveValue("b");
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(cancel).toHaveBeenCalledOnce();
  expect(mocks.put).toHaveBeenCalledOnce();
});
