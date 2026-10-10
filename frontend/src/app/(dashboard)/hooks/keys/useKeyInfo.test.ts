import React, { type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { useKeyInfo } from "./useKeyInfo";

const mocks = vi.hoisted(() => ({ auth: vi.fn(), fetch: vi.fn(), info: vi.fn() }));
vi.mock("../useAuthorized", () => ({ default: mocks.auth }));
vi.mock("@/components/networking", () => ({
  getProxyBaseUrl: () => "",
  getGlobalLitellmHeaderName: () => "Authorization",
  keyInfoV1Call: mocks.info,
}));
let client: QueryClient;
/** 初始化独立查询缓存和认证模拟；供各测试使用，无返回值，框架清理 DOM，测试不写数据库。 */
beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("fetch", mocks.fetch);
  mocks.auth.mockReturnValue({ accessToken: "session", userId: "me" });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
/** 为 hook 提供独立缓存；参数 children 为测试节点，返回 Provider，无持久副作用。 */
const wrapper = ({ children }: { children: ReactNode }) =>
  React.createElement(QueryClientProvider, { client }, children);

/** 验证本人详情契约；前置有效身份和接口响应，确认范围参数与 URL 转义，清理由框架执行。 */
it("requests personal detail and maps the response", async () => {
  mocks.fetch.mockResolvedValueOnce({ ok: true, text: async () => JSON.stringify({ info: { key_alias: "mine" } }) });
  const { result } = renderHook(() => useKeyInfo("id&other", { scope: "personal" }), { wrapper });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(mocks.fetch).toHaveBeenCalledWith("/key/info?key=id%26other&scope=personal", {
    method: "GET", headers: { "Content-Type": "application/json", Authorization: "Bearer session" }, signal: undefined, credentials: undefined,
  });
  expect(result.current.data).toMatchObject({ token: "id&other", key_alias: "mine" });
  expect(mocks.info).not.toHaveBeenCalled();
});

/** 验证越界详情失败不会回退管理接口；前置后台 404，结果无密钥数据；框架销毁缓存。 */
it("does not fall back to unrestricted detail after a personal 404", async () => {
  mocks.fetch.mockResolvedValueOnce({ ok: false, status: 404, text: async () => "Not found" });
  const { result } = renderHook(() => useKeyInfo("foreign", { scope: "personal" }), { wrapper });
  await waitFor(() => expect(result.current.isError).toBe(true));
  expect(result.current.data).toBeUndefined();
  expect(mocks.info).not.toHaveBeenCalled();
});

/** 验证空 ID、未登录及显式禁用不查询；参数为边界组合，断言无网络，框架销毁 DOM。 */
it.each([ [null, "session", true], ["id", null, true], ["id", "session", false] ] as const)(
  "does not request disabled personal detail (%s, %s, %s)", (id, token, enabled) => {
    mocks.auth.mockReturnValue({ accessToken: token });
    renderHook(() => useKeyInfo(id, { scope: "personal", enabled }), { wrapper });
    expect(mocks.fetch).not.toHaveBeenCalled();
  },
);

/** 验证管理页面沿用原详情接口；前置有效会话，断言无个人网络请求；框架清理缓存。 */
it("preserves management detail queries", async () => {
  mocks.info.mockResolvedValueOnce({ info: { key_alias: "managed" } });
  const { result } = renderHook(() => useKeyInfo("managed-id"), { wrapper });
  await waitFor(() => expect(result.current.isSuccess).toBe(true));
  expect(mocks.info).toHaveBeenCalledWith("session", "managed-id");
  expect(mocks.fetch).not.toHaveBeenCalled();
});
