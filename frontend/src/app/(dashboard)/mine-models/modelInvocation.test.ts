import { describe, expect, it } from "vitest";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { invocationCurl, invocationURL } from "./modelInvocation";

const endpoint: ModelEndpoint = {
  endpoint_id: "chat",
  transport: "bypass_openai_chat",
  kind: "adapted",
  protocol: "openai-chat",
  family: "chat",
  method: "POST",
  path: "/v1/chat/completions",
};

describe("我的模型调用示例", () => {
  /** 前置绝对网关或带代理前缀地址；验证正确拼接与非法输入拒绝；纯函数无数据清理。 */
  it("保留代理前缀、避免重复 v1，并拒绝非法 URL", () => {
    expect(invocationURL("https://gateway.test/proxy/v1/", endpoint.path)).toBe(
      "https://gateway.test/proxy/v1/chat/completions",
    );
    expect(invocationURL("http://127.0.0.1:4000/", "/v1/responses")).toBe("http://127.0.0.1:4000/v1/responses");
    for (const base of ["", "invalid", "javascript:alert(1)"]) expect(invocationURL(base, endpoint.path)).toBeNull();
    for (const path of ["https://evil.test", "//evil.test", ""])
      expect(invocationURL("https://gateway.test", path)).toBeNull();
  });
  /** 前置后台开放的六种 JSON 协议；验证公开别名、协议正文和认证头；无网络与持久数据。 */
  it.each(["openai-chat", "openai-responses", "anthropic-messages", "gemini", "vertex", "openai-embeddings"])(
    "生成 %s 的真实协议示例",
    (protocol) => {
      const sample = invocationCurl("my-model", { ...endpoint, protocol }, "https://gateway.test")!;
      expect(sample).toContain("https://gateway.test/v1/chat/completions");
      expect(sample).toContain("Authorization: Bearer $XHUB_API_KEY");
      if (["gemini", "vertex"].includes(protocol)) {
        expect(sample).toContain('"contents"');
        expect(sample).not.toContain('"model"');
      } else expect(sample).toContain('"model": "my-model"');
      if (protocol === "anthropic-messages") expect(sample).toContain("anthropic-version: 2023-06-01");
      if (protocol === "openai-responses") expect(sample).toContain('"input": "你好"');
      expect(sample).toContain(" \\\n");
    },
  );
  /** 前置特殊字符及专用媒体、错误方法；验证 shell 转义与不猜测参数；纯函数无需清理。 */
  it("转义单引号并拒绝专用协议、空别名和错误方法", () => {
    const sample = invocationCurl("model'$(echo test)", endpoint, "https://gateway.test")!;
    expect(sample).toContain("model'\\''$(echo test)");
    for (const protocol of ["fal", "openai-videos", "openai-images", "unknown"])
      expect(invocationCurl("model", { ...endpoint, protocol }, "https://gateway.test")).toBeNull();
    expect(invocationCurl(" ", endpoint, "https://gateway.test")).toBeNull();
    expect(invocationCurl("model", { ...endpoint, method: "GET" }, "https://gateway.test")).toBeNull();
  });
});
