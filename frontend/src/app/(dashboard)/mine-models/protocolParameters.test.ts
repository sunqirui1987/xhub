import { expect, it } from "vitest";
import { protocolParameters } from "./protocolParameters";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { zhCN as zh } from "@/i18n/messages/zh-CN";
import { en } from "@/i18n/messages/en";

/** 前置已知/未知协议和两种接入方式；验证参数文档、翻译完整和 Google 路径模型边界；纯函数无清理。 */
it.each(["openai-chat", "openai-responses", "anthropic-messages", "gemini", "vertex", "ark", "fal", "openai-images", "openai-videos", "openai-embeddings", "openai-audio-speech", "openai-audio-transcription", "openai-audio-translation", "unknown"])("%s 普通与 Bypass 共用完整翻译参数", (protocol) => {
  const endpoint: ModelEndpoint = { endpoint_id: "test", transport: "test", kind: "adapted", method: "POST", protocol, family: "test", path: "/test" };
  const rows = protocolParameters(endpoint);
  expect(rows).toEqual(protocolParameters({ ...endpoint, kind: "bypass" }));
  for (const row of rows) {
    expect(zh.myModels).toHaveProperty(row.description);
    expect(en.myModels).toHaveProperty(row.description);
  }
  expect(rows[1].field).toBe(["gemini", "vertex"].includes(protocol) ? "model (URL)" : "model");
  if (protocol === "unknown") expect(rows.at(-1)?.field).toBe("request.json");
});
