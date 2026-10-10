import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n/runtime";
import { CurlRequestGuide } from "./CurlRequestGuide";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
const endpoint: ModelEndpoint = {
  endpoint_id: "ark",
  kind: "bypass",
  transport: "ark",
  protocol: "ark",
  family: "video",
  method: "POST",
  path: "/v3/contents/generations/tasks",
  actions: [{ name: "get", method: "GET", public_path: "/v3/contents/generations/tasks/{id}" }],
};
beforeEach(() => setActiveLocale("zh-CN"));

/** 前置真实正文和任务 ID；验证步骤复制、正文更新及失败提示，DOM 自动清理，恢复剪贴板替身。 */
it("完整 curl 随当前正文更新，复制失败可观察", async () => {
  const user = userEvent.setup();
  const props = {
    endpoint,
    model: "public-video",
    base: "https://gateway.test",
    body: '{"content":[{"type":"text","text":"A cat"}]}',
    taskId: "task'123",
  };
  const { rerender } = render(<CurlRequestGuide {...props} />);
  fireEvent.click(screen.getByText("完整 curl 调用"));
  expect(screen.getByLabelText("复制任务 ID 设置", { selector: "pre" })).toHaveTextContent("task'\\''123");
  await user.click(screen.getByRole("button", { name: "复制调用示例" }));
  expect(await navigator.clipboard.readText()).toContain('"model": "public-video"');
  rerender(<CurlRequestGuide {...props} body='{"prompt":"A dog", "duration":5}' />);
  expect(screen.getByLabelText("复制调用示例", { selector: "pre" })).toHaveTextContent('"duration": 5');
  vi.spyOn(navigator.clipboard, "writeText").mockRejectedValueOnce(new Error("denied"));
  await user.click(screen.getByRole("button", { name: "复制调用示例" }));
  expect(screen.getByRole("alert")).toHaveTextContent("复制失败");
  vi.restoreAllMocks();
});

/** 前置无效 JSON、数组或网关；验证没有可复制的错误请求；DOM 自动清理。 */
it.each(["{", "[]", "null"])("拒绝无效正文 %s", (body) => {
  render(<CurlRequestGuide endpoint={endpoint} model="video" base="https://gateway.test" body={body} />);
  fireEvent.click(screen.getByText("完整 curl 调用"));
  expect(screen.getByRole("alert")).toHaveTextContent("有效 JSON 对象");
  expect(screen.queryByRole("button", { name: "复制调用示例" })).not.toBeInTheDocument();
});

/** 前置英文语言与有效原生正文；验证教程和文档均翻译、英文示例可复制；DOM 自动清理。 */
it("英文界面使用翻译后的步骤和协议文档", () => {
  setActiveLocale("en");
  render(
    <CurlRequestGuide
      endpoint={endpoint}
      model="video"
      base="https://gateway.test"
      body='{"content":[{"type":"text","text":"A cat"}]}'
    />,
  );
  fireEvent.click(screen.getByText("Complete curl request"));
  expect(screen.getByRole("heading", { name: "2. Create a generation task" })).toBeInTheDocument();
  fireEvent.click(screen.getByText("Protocol parameters"));
  expect(screen.getByText(/Required array: text/)).toBeInTheDocument();
});
