import { describe, expect, it } from "vitest";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { invocationActionCurl, invocationCurl, invocationSteps, invocationURL } from "./modelInvocation";

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
      if (protocol === "openai-responses") expect(sample).toContain('"input": "Hello"');
      expect(sample).toContain(" \\\n");
    },
  );
  /** 前置特殊字符及专用媒体、错误方法；验证 shell 转义与不猜测参数；纯函数无需清理。 */
  it("转义单引号、覆盖媒体协议并拒绝空别名和错误方法", () => {
    const sample = invocationCurl("model'$(echo test)", endpoint, "https://gateway.test")!;
    expect(sample).toContain("model'\\''$(echo test)");
    for (const protocol of ["fal", "openai-videos", "openai-images", "unknown"])
      expect(invocationCurl("model", { ...endpoint, protocol }, "https://gateway.test")).toContain(
        "curl --request POST",
      );
    expect(invocationCurl(" ", endpoint, "https://gateway.test")).toBeNull();
    expect(invocationCurl("model", { ...endpoint, method: "GET" }, "https://gateway.test")).toBeNull();
  });
});

/** 前置异步协议与代理前缀；验证查询排序、查询参数、删除和下载，非法动作拒绝；纯函数无清理。 */
it("异步任务按创建、保存 ID、状态、结果、下载和删除排序", () => {
  const video = {
    ...endpoint,
    protocol: "fal",
    path: "/queue/text-to-video",
    actions: [
      { name: "get", method: "GET", public_path: "/queue/{request_id}" },
      { name: "status", method: "GET", public_path: "/queue/{request_id}/status" },
    ],
  };
  expect(invocationSteps("video", video, "https://gateway.test").map((step) => step.title)).toEqual([
    "prepareKey",
    "createTask",
    "saveTaskId",
    "queryStatus",
    "queryResult",
    "downloadVideo",
  ]);
  expect(
    invocationActionCurl(
      { name: "get", method: "GET", public_path: "/tasks", task_query: "taskId" },
      "https://gateway.test",
    ),
  ).toContain("--data-urlencode 'taskId='\"$TASK_ID\"");
  expect(
    invocationActionCurl({ name: "cancel", method: "DELETE", public_path: "/tasks/{id}" }, "https://gateway.test"),
  ).toContain("curl --request DELETE");
  for (const action of [
    { method: "POST", public_path: "/tasks/{id}" },
    { method: "GET", public_path: "/tasks/{unknown}" },
    { method: "GET", public_path: "//evil.test" },
  ])
    expect(invocationActionCurl({ name: "get", ...action }, "https://gateway.test")).toBeNull();
  expect(invocationSteps("video", video, "invalid")).toEqual([]);
  const ark = {
    ...endpoint,
    protocol: "ark",
    path: "/v3/contents/generations/tasks",
    actions: [
      { name: "get", method: "GET", public_path: "/v3/contents/generations/tasks/{id}" },
      { name: "delete", method: "DELETE", public_path: "/v3/contents/generations/tasks/{id}" },
    ],
  };
  expect(invocationSteps("video", ark, "https://gateway.test").map((step) => step.title)).toEqual([
    "prepareKey",
    "createTask",
    "saveTaskId",
    "queryResult",
    "downloadVideo",
    "deleteTask",
  ]);
});

/** 前置实时 JSON 和含 shell、multipart 特殊字符文件名；验证公开别名覆盖、完整参数、流式与秘密排除；无网络清理。 */
it("当前参数完整序列化，上传路径和 shell 各自转义", () => {
  const live = invocationCurl("public", endpoint, "https://gateway.test", {
    body: {
      model: "wrong",
      messages: [{ role: "user", content: "it's $(touch /tmp/never)" }],
      stream: true,
      temperature: 0.2,
    },
    headers: { Authorization: "secret", "x-api-key": "secret", "x-litellm-tags": "demo" },
  })!;
  expect(live).toContain('"model": "public"');
  expect(live).toContain('"temperature": 0.2');
  expect(live).toContain("--no-buffer");
  expect(live).not.toContain("secret");
  const upload = invocationCurl(
    "public",
    { ...endpoint, protocol: "openai-images", path: "/v1/images/edits" },
    "https://gateway.test",
    { body: { prompt: "Paint it", size: "1024x1024" }, multipart: true, files: { image: "cat;$(echo hi).png" } },
  )!;
  expect(upload).toContain("--form 'image=@\"./cat;$(echo hi).png\"'");
  expect(upload).toContain("--form-string 'size=1024x1024'");
  expect(upload).not.toContain("Content-Type");
  expect(invocationCurl("public", { ...endpoint, protocol: "unknown" }, "https://gateway.test")).toContain(
    "--data @./request.json",
  );
});

/** 前置全部媒体示例协议；验证英文提示词、正文与文件准备约定；无持久数据。 */
it.each([
  "ark",
  "fal",
  "openai-images",
  "openai-videos",
  "openai-audio-speech",
  "openai-audio-transcription",
  "openai-audio-translation",
  "rerank",
  "openai-completions",
])("%s 示例可复制且提示词为英文", (protocol) => {
  const command = invocationCurl(
    "public",
    { ...endpoint, protocol, path: protocol === "fal" ? "/queue/text-to-video" : endpoint.path },
    "https://gateway.test",
  )!;
  expect(command).toContain("--fail-with-body");
  expect(command).not.toMatch(/[\u4e00-\u9fff]/);
  if (protocol.includes("transcription") || protocol.includes("translation"))
    expect(command).toContain("file=@./audio.mp3");
  if (protocol === "openai-audio-speech") expect(command).toContain("--output ./speech.mp3");
});
