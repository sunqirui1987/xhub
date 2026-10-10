import { afterEach, expect, it, vi } from "vitest";
import { requestDocumentation } from "./documentation";
/** 每例恢复网络边界；无参数返回值，避免污染其他单元测试，无业务数据。 */
afterEach(() => vi.unstubAllGlobals());
/** 目的：保留错误状态和二进制响应；前置替身返回 401，原样返回响应且不注入会话，不写数据库。 */
it("preserves raw responses and explicit request options", async () => {
  const response = new Response("denied", { status: 401 });
  const fetch = vi.fn().mockResolvedValue(response);
  vi.stubGlobal("fetch", fetch);
  const options: RequestInit = { method: "GET", credentials: "omit" };
  expect(await requestDocumentation("https://gateway.example/models", options)).toBe(response);
  expect(fetch).toHaveBeenCalledExactlyOnceWith("https://gateway.example/models", options);
});
/** 目的：网络中断不自动重试；前置请求被取消或离线，错误交由运行器展示，一次请求后清理替身。 */
it("propagates network failures without retries", async () => {
  const fetch = vi.fn().mockRejectedValue(new Error("offline"));
  vi.stubGlobal("fetch", fetch);
  await expect(requestDocumentation("https://gateway.example/models", {})).rejects.toThrow("offline");
  expect(fetch).toHaveBeenCalledTimes(1);
});
