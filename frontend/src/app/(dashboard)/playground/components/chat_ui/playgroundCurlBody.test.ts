import { expect, it } from "vitest";
import { playgroundCurlBody, type CurlBodyOptions } from "./playgroundCurlBody";
const options: CurlBodyOptions = {
  endpoint: {
    endpoint_id: "chat",
    transport: "test",
    kind: "adapted",
    method: "POST",
    protocol: "openai-chat",
    family: "chat",
    path: "/v1/chat/completions",
  },
  input: "Hello",
  messages: [{ role: "user", content: "Hello" }],
  stream: true,
  base: "https://gateway.test",
  temperature: 0.3,
  maxTokens: 300,
  guardrails: ["guard"],
  policies: ["policy"],
  vectorStores: ["store"],
};

/** 前置标准聊天设置；验证参数同步，Bypass 不携带网关扩展或未发送的设置；无数据清理。 */
it("聊天正文同步当前设置并区分 Bypass", () => {
  expect(playgroundCurlBody(options)).toMatchObject({
    messages: options.messages,
    temperature: 0.3,
    max_tokens: 300,
    guardrails: ["guard"],
    stream_options: { include_usage: true },
  });
  expect(playgroundCurlBody({ ...options, endpoint: { ...options.endpoint, kind: "bypass" } })).toEqual({
    messages: options.messages,
    stream: true,
  });
  expect(playgroundCurlBody({ ...options, messages: [], input: "" })).toMatchObject({
    messages: [{ role: "user", content: "Hello" }],
  });
});
/** 前置 Responses 续接和 Messages；验证各自字段、系统提示及非流式边界；无网络与清理。 */
it("Responses 续接与 Messages 各自使用协议字段", () => {
  expect(
    playgroundCurlBody({
      ...options,
      endpoint: { ...options.endpoint, protocol: "openai-responses" },
      previousResponseId: "resp-1",
      messages: [{ role: "assistant", content: "Old" }, ...options.messages],
    }),
  ).toMatchObject({ previous_response_id: "resp-1", input: [{ role: "user", content: "Hello", type: "message" }] });
  expect(
    playgroundCurlBody({
      ...options,
      endpoint: { ...options.endpoint, protocol: "anthropic-messages", kind: "bypass" },
      messages: [{ role: "system", content: "Be brief" }, ...options.messages],
      stream: false,
    }),
  ).toEqual({ system: "Be brief", messages: options.messages, max_tokens: 2048, stream: false });
});
/** 前置媒体协议；验证图片、音频和向量真实表单正文，multipart 文件由上层单独提供；无清理。 */
it.each(["openai-images", "openai-audio-speech", "openai-audio-transcription", "openai-embeddings"])(
  "%s 使用对应输入字段",
  (protocol) => {
    const body = playgroundCurlBody({ ...options, endpoint: { ...options.endpoint, protocol } });
    expect(body).not.toHaveProperty("messages");
    expect(body).not.toHaveProperty("guardrails");
    if (protocol === "openai-images") expect(body).toEqual({ prompt: "Hello" });
    if (protocol === "openai-audio-speech") expect(body).toEqual({ input: "Hello", voice: "alloy" });
  },
);

/** 前置两种图片地址及 Bypass Messages；验证实际协议转换和 Responses 内容类型，原始消息不变，无持久数据。 */
it("原生多模态按实际协议转换图片", () => {
  const messages = [
    {
      role: "user",
      content: [
        { type: "text", text: "Describe" },
        { type: "image_url", image_url: { url: "data:image/png;base64,AAAA" } },
        { type: "image_url", image_url: { url: "https://example.com/image.png" } },
      ],
    },
  ];
  const body = playgroundCurlBody({
    ...options,
    messages,
    endpoint: { ...options.endpoint, protocol: "anthropic-messages", kind: "bypass" },
  });
  expect(body.messages).toEqual([
    {
      role: "user",
      content: [
        { type: "text", text: "Describe" },
        { type: "image", source: { type: "base64", media_type: "image/png", data: "AAAA" } },
        { type: "image", source: { type: "url", url: "https://example.com/image.png" } },
      ],
    },
  ]);
  expect(messages[0].content[1].type).toBe("image_url");
  expect(
    playgroundCurlBody({
      ...options,
      messages,
      endpoint: { ...options.endpoint, protocol: "openai-responses", kind: "bypass" },
    }).input,
  ).toEqual([
    {
      role: "user",
      content: [
        { type: "input_text", text: "Describe" },
        { type: "input_image", image_url: "data:image/png;base64,AAAA" },
        { type: "input_image", image_url: "https://example.com/image.png" },
      ],
    },
  ]);
});
/** 前置 Responses 工具与代码解释器；验证工具路径、允许列表及 Bypass 扩展过滤，无外部工具调用或清理。 */
it("工具配置跟随协议且原生请求不注入工具", () => {
  const configured = {
    ...options,
    endpoint: { ...options.endpoint, protocol: "openai-responses" },
    selectedMCPServers: ["__all__"],
    codeInterpreter: true,
  };
  expect(playgroundCurlBody(configured)).toMatchObject({
    tools: [
      { type: "mcp", server_url: "https://gateway.test/mcp" },
      { type: "code_interpreter", container: { type: "auto" } },
    ],
    tool_choice: "auto",
  });
  expect(
    playgroundCurlBody({ ...configured, endpoint: { ...configured.endpoint, kind: "bypass" } }),
  ).not.toHaveProperty("tools");
});
