import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { MyModelDetail } from "./MyModelDetail";
import { MyModels } from "./MyModels";
import type { MyModelCard } from "./grantedModelCards";

vi.mock("@/components/networking", () => ({ getProxyBaseUrl: () => "https://gateway.test" }));
const model: MyModelCard = {
  id: "my-chat",
  category: "chat",
  provider: "openai",
  capabilities: [],
  max_input_tokens: null,
  max_output_tokens: null,
  input_price: 0,
  output_price: 2,
  cache_read_price: null,
  cache_write_price: 3,
  endpoints: [
    {
      endpoint_id: "chat",
      transport: "bypass_openai_chat",
      kind: "adapted",
      protocol: "openai-chat",
      family: "chat",
      method: "POST",
      path: "/v1/chat/completions",
    },
    {
      endpoint_id: "responses",
      transport: "bypass_openai_chat",
      kind: "adapted",
      protocol: "openai-responses",
      family: "chat",
      method: "POST",
      path: "/v1/responses",
    },
  ],
};
beforeEach(() => setActiveLocale("zh-CN"));

/** 前置授权聊天模型；验证卡片四入口、默认 API、协议切换、复制及关闭焦点；DOM自动清理，无持久数据。 */
it("从卡片直接打开四标签详情并复制实际接口示例", async () => {
  const user = userEvent.setup();
  render(<MyModels models={[model]} />);
  for (const name of ["模型价格", "接入信息", "API 接入", "调用文档"])
    expect(screen.getByRole("button", { name, exact: true })).toBeInTheDocument();
  const trigger = screen.getByRole("button", { name: "如何调用", exact: true });
  await user.click(trigger);
  const dialog = screen.getByRole("dialog", { name: "my-chat" });
  expect(within(dialog).getByRole("tab", { name: "API 接入" })).toHaveAttribute("aria-selected", "true");
  expect(within(dialog).queryByRole("link", { name: "查看 API 接口定义" })).not.toBeInTheDocument();
  expect(within(dialog).getByRole("link", { name: "管理虚拟密钥" })).toHaveAttribute("href", expect.stringContaining("api-keys"));
  await user.selectOptions(screen.getByRole("combobox", { name: "选择调用接口" }), "/v1/responses");
  const code = screen.getByLabelText("复制调用示例", { selector: "pre" });
  expect(code).toHaveTextContent('"input": "Hello"');
  expect(code).toHaveTextContent("https://gateway.test/v1/responses");
  await user.click(screen.getByRole("button", { name: "复制调用示例" }));
  expect(await navigator.clipboard.readText()).toBe(code.textContent);
  expect(screen.getByRole("status")).toHaveTextContent("已复制");
  await user.click(screen.getByRole("tab", { name: "模型价格" }));
  const prices = screen.getByRole("table", { name: "模型价格" });
  expect(within(prices).getByText("$0")).toBeInTheDocument();
  expect(within(prices).getByText("价格未提供")).toBeInTheDocument();
  await user.click(screen.getByRole("tab", { name: "接入信息" }));
  expect(screen.getByText("POST /v1/chat/completions")).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "复制模型名称" }));
  expect(await navigator.clipboard.readText()).toBe(model.id);
  await user.click(screen.getByRole("button", { name: "复制网关地址" }));
  expect(await navigator.clipboard.readText()).toBe("https://gateway.test");
  await user.click(screen.getByRole("tab", { name: "调用文档" }));
  expect(screen.getByRole("heading", { name: "1. 准备 API Key" })).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "API 接入", exact: true }));
  expect(within(dialog).getByRole("tab", { name: "API 接入" })).toHaveAttribute("aria-selected", "true");
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(trigger).toHaveFocus();
});

/** 前置剪贴板拒绝写入；验证失败提示和示例正文保留，测试替身恢复且DOM自动清理。 */
it("复制失败可观察且保留手动复制内容", async () => {
  const user = userEvent.setup();
  vi.spyOn(navigator.clipboard, "writeText").mockRejectedValueOnce(new Error("denied"));
  render(<MyModelDetail model={model} initialTab="api" onClose={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "复制调用示例" }));
  expect(screen.getByRole("alert")).toHaveTextContent("复制失败");
  expect(screen.getByLabelText("复制调用示例", { selector: "pre" })).toHaveTextContent("curl --request POST");
  vi.restoreAllMocks();
});

/** 前置缺失、空绑定或无效部署；验证不误报可用且不展示聊天示例；DOM自动清理。 */
it.each([undefined, [], model.endpoints])("未开放或配置失败时保留说明而不猜测示例 %j", (endpoints) => {
  render(
    <MyModelDetail
      model={{ ...model, endpoints, ...(endpoints?.length ? { unavailable_reason: "invalid transport" } : {}) }}
      initialTab="api"
      onClose={vi.fn()}
    />,
  );
  expect(screen.getByRole("alert")).toHaveTextContent("暂未提供可调用接口");
  expect(screen.queryByRole("link", { name: "查看 API 接口定义" })).not.toBeInTheDocument();
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "复制调用示例" })).not.toBeInTheDocument();
});

/** 前置视频绑定；验证创建、任务 ID、查询、下载和参数文档完整可复制；DOM 自动清理。 */
it("视频协议展示顺序 curl 和参数说明", async () => {
  const user = userEvent.setup();
  render(<MyModelDetail model={{ ...model, endpoints: [{ ...model.endpoints![0], kind: "bypass", protocol: "ark", family: "video", path: "/v3/contents/generations/tasks", actions: [{ name: "get", method: "GET", public_path: "/v3/contents/generations/tasks/{id}" }] }] }} initialTab="api" onClose={vi.fn()} />);
  expect(screen.getByLabelText("复制调用示例", { selector: "pre" })).toHaveTextContent('"content"');
  expect(screen.getByLabelText("复制结果查询", { selector: "pre" })).toHaveTextContent('"$TASK_ID"');
  expect(screen.getByLabelText("复制下载命令", { selector: "pre" })).toHaveTextContent("--output ./video.mp4");
  await user.click(screen.getByRole("button", { name: "查看协议参数" }));
  expect(screen.getByRole("table", { name: "协议参数说明" })).toHaveTextContent("content.video_url");
  await user.click(screen.getByRole("button", { name: "返回 curl 步骤" }));
  expect(screen.getByRole("heading", { name: "2. 创建生成任务" })).toBeInTheDocument();
});
