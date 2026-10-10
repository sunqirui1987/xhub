import { describe, it, expect } from "vitest";
import { nativeRequestTemplate } from "./nativeRequestTemplate";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
/** 构造声明绑定；参数协议及路径，返回本地夹具；仅本测试调用，无外部写入。 */
const binding = (protocol: string, path = "/test"): ModelEndpoint => ({
  endpoint_id: protocol,
  protocol,
  path,
  kind: "bypass",
  transport: protocol,
  method: "POST",
  family: "text",
});
describe("原生参数模板", () => {
  /** 前置各已知协议；验证协议字段完整且不注入模型或密钥；无清理数据。 */
  it.each([
    ["gemini", "contents"],
    ["vertex", "contents"],
    ["openai-responses", "input"],
    ["anthropic-messages", "messages"],
  ])("生成 %s 原生字段", (protocol, field) => {
    const doc = JSON.parse(nativeRequestTemplate(binding(protocol)));
    expect(doc).toHaveProperty(field);
    expect(doc).not.toHaveProperty("model");
    expect(doc).not.toHaveProperty("api_key");
  });
  /** 前置 Ark、图片与未知路径；验证视频 content、图片 prompt 及未知协议空对象降级，无清理数据。 */
  it("视频和图片不复用错误的通用模板", () => {
    expect(JSON.parse(nativeRequestTemplate(binding("ark", "/v3/contents/generations/tasks")))).toHaveProperty(
      "content",
    );
    expect(JSON.parse(nativeRequestTemplate(binding("image", "/images/generations")))).toEqual({ prompt: "" });
    expect(JSON.parse(nativeRequestTemplate(binding("unknown")))).toEqual({});
  });
});
