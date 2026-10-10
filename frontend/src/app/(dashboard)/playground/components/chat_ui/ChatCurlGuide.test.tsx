import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n";
import { ChatCurlGuide } from "./ChatCurlGuide";
import { createChatMultimodalMessage } from "./ChatImageUtils";
import type { CurlBodyOptions } from "./playgroundCurlBody";
vi.mock("./ChatImageUtils", () => ({ createChatMultimodalMessage: vi.fn() }));
const options: CurlBodyOptions & { model: string } = {
  model: "public-chat",
  endpoint: {
    endpoint_id: "chat",
    kind: "adapted",
    transport: "test",
    protocol: "openai-chat",
    family: "chat",
    method: "POST",
    path: "/v1/chat/completions",
  },
  input: "用户自己的输入",
  messages: [],
  stream: false,
  base: "https://gateway.test",
};
beforeEach(() => {
  setActiveLocale("zh-CN");
  vi.resetAllMocks();
});
/** 前置当前草稿、历史及标签；验证教程保存用户语言、正文同步和空输入英文示例，自动卸载，无网络或密钥。 */
it("聊天 curl 保存输入并同步历史及标签", () => {
  const { rerender } = render(<ChatCurlGuide options={options} tags={["review"]} />);
  fireEvent.click(screen.getByText("完整 curl 调用"));
  const curl = screen.getByLabelText("复制调用示例", { selector: "pre" });
  expect(curl).toHaveTextContent("用户自己的输入");
  expect(curl).toHaveTextContent("x-litellm-tags: review");
  rerender(
    <ChatCurlGuide options={{ ...options, input: "", messages: [{ role: "user", content: "Earlier" }] }} tags={[]} />,
  );
  expect(curl).toHaveTextContent("Earlier");
  expect(curl).not.toHaveTextContent("用户自己的输入");
  rerender(<ChatCurlGuide options={{ ...options, input: "" }} tags={[]} />);
  expect(curl).toHaveTextContent('"content": "Hello"');
});
/** 前置异步附件读取；验证读取前不能复制，完成后含编码图片，自动卸载无持久数据。 */
it("图片读取完成才生成可复制请求", async () => {
  let resolve: (value: Awaited<ReturnType<typeof createChatMultimodalMessage>>) => void = () => {};
  vi.mocked(createChatMultimodalMessage).mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const file = new File(["image"], "cat.png", { type: "image/png" });
  render(<ChatCurlGuide options={options} tags={[]} image={file} />);
  fireEvent.click(screen.getByText("完整 curl 调用"));
  expect(screen.queryByRole("button", { name: "复制调用示例" })).not.toBeInTheDocument();
  resolve({
    role: "user",
    content: [
      { type: "text", text: options.input },
      { type: "image_url", image_url: { url: "data:image/png;base64,AAAA" } },
    ],
  });
  await waitFor(() =>
    expect(screen.getByLabelText("复制调用示例", { selector: "pre" })).toHaveTextContent("data:image/png;base64,AAAA"),
  );
});
/** 前置附件读取失败；验证明确不可复制的降级，避免遗漏附件请求，自动卸载无网络清理。 */
it("附件读取失败不显示错误请求", async () => {
  vi.mocked(createChatMultimodalMessage).mockRejectedValue(new Error("read failed"));
  render(<ChatCurlGuide options={options} tags={[]} image={new File(["x"], "bad.png")} />);
  fireEvent.click(screen.getByText("完整 curl 调用"));
  await waitFor(() => expect(screen.getByRole("alert")).toBeVisible());
  expect(screen.queryByRole("button", { name: "复制调用示例" })).not.toBeInTheDocument();
});
/** 前置多张图片编辑；验证每张图独立 multipart 命令，文件名安全引用而非浏览器二进制，自动卸载无数据。 */
it("每张编辑图片生成对应 multipart curl", () => {
  render(
    <ChatCurlGuide
      options={{ ...options, endpoint: { ...options.endpoint, protocol: "openai-images", path: "/v1/images/edits" } }}
      tags={[]}
      files={[new File(["1"], "first.png"), new File(["2"], "second.png")]}
    />,
  );
  for (const summary of screen.getAllByText("完整 curl 调用")) fireEvent.click(summary);
  const curls = screen.getAllByLabelText("复制调用示例", { selector: "pre" });
  expect(curls).toHaveLength(2);
  expect(curls[0]).toHaveTextContent("first.png");
  expect(curls[1]).toHaveTextContent("second.png");
  expect(curls[0]).toHaveTextContent("--form");
});
