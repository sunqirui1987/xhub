import { describe, expect, it } from "vitest";
import { fullErrorDetails, isRequestFailure } from "./logErrors";

describe("request error diagnostics", () => {
  /** 前置成功、旧版和后台失败状态及 HTTP 错误；验证统一判定边界，纯函数无需清理。 */
  it.each([
    [{ status: "success" }, false], [{}, false], [{ error: " " }, false],
    [{ status: "error" }, true], [{ metadata: { status: "failure" } }, true],
    [{ status: "FAILED" }, true], [{ metadata: { http_status: 400 } }, true],
    [{ metadata: { http_status: 399 } }, false], [{ error: "connection refused" }, true],
  ])("recognizes %j as failure=%s", (log, expected) => {
    expect(isRequestFailure(log)).toBe(expected);
  });
  /** 前置完整长 JSON、纯文本、旧版摘要及空记录；验证无截断和正文优先级，纯函数无需清理。 */
  it("preserves complete diagnostics including provider extensions", () => {
    const error = '{"error":{"message":"' + "x".repeat(2048) + '","details":{"trace":"tail-marker"}}}';
    expect(fullErrorDetails({ error, response: { error: "short" } })).toBe(error);
    expect(fullErrorDetails({ response: "non-json failure\nlast line" })).toBe("non-json failure\nlast line");
    expect(fullErrorDetails({ response: { error: { details: { trace: "tail-marker" } } } })).toContain("tail-marker");
    expect(fullErrorDetails({ response: {}, metadata: { error_information: { error_message: "legacy" } } })).toContain("legacy");
    expect(fullErrorDetails({ response: {}, error: " " })).toBe("");
  });
});
