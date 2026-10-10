import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider } from "@/i18n/I18nProvider";
import { ApiRunner } from "./ApiRunner";
import LanguageSwitcher from "@/components/LanguageSwitcher";
import { LOCALE_COOKIE } from "@/i18n/translate";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));

/** 清理请求替身及语言状态；无参数返回值，供每个场景结束调用，不写业务数据。 */
afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  document.cookie = LOCALE_COOKIE + "=; path=/; max-age=0";
});
/** 创建双语运行器；参数为语言，返回真实组件树，凭据仅留内存，框架负责卸载。 */
function setup(locale: "en" | "zh-CN" = "en") {
  localStorage.clear();
  document.cookie = LOCALE_COOKIE + "=; path=/; max-age=0";
  return render(
    <I18nProvider initialLocale={locale}>
      <LanguageSwitcher />
      <ApiRunner
        base="https://gateway.example"
        endpoint="/v1/chat/completions"
        method="POST"
        initialBody='{"model":"example","messages":[{"role":"user","content":"Hello"}]}'
      />
    </I18nProvider>,
  );
}
/** 目的：验证用户主动运行、认证及结果；前置英文组件和成功请求边界，核对真实请求形状、状态及凭据不持久化，测试后清理替身。 */
it("runs JSON only after a click and shows the HTTP response", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(
      new Response('{"usage":{"total_tokens":3}}', { headers: { "content-type": "application/json" } }),
    );
  vi.stubGlobal("fetch", fetch);
  setup();
  expect(fetch).not.toHaveBeenCalled();
  expect(screen.getByLabelText("XHub API key")).toHaveAttribute("autocomplete", "new-password");
  expect(screen.getByRole("form", { name: "Run online" })).toHaveAttribute("autocomplete", "off");
  fireEvent.change(screen.getByLabelText("XHub API key"), { target: { value: "sk-docs-private" } });
  fireEvent.click(screen.getByRole("button", { name: "Run request" }));
  expect(await screen.findByRole("status")).toHaveTextContent("HTTP 200");
  expect(fetch).toHaveBeenCalledWith(
    "https://gateway.example/v1/chat/completions",
    expect.objectContaining({
      method: "POST",
      headers: { Authorization: "Bearer sk-docs-private", "Content-Type": "application/json" },
      credentials: "omit",
    }),
  );
  expect(JSON.stringify(localStorage)).not.toContain("sk-docs-private");
});
/** 目的：验证路径与正文边界；前置已填密钥，外站路径和数组正文必须在发请求前报错，测试后清理替身。 */
it("rejects invalid paths and non-object JSON without a request", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  setup();
  fireEvent.change(screen.getByLabelText("XHub API key"), { target: { value: "key" } });
  fireEvent.change(screen.getByLabelText("Request path"), { target: { value: "//other.example" } });
  fireEvent.click(screen.getByRole("button", { name: "Run request" }));
  expect(screen.getByRole("alert")).toHaveTextContent("valid path");
  fireEvent.change(screen.getByLabelText("Request path"), { target: { value: "/v1/chat/completions" } });
  fireEvent.change(screen.getByLabelText("Request body (JSON)"), { target: { value: "[]" } });
  fireEvent.click(screen.getByRole("button", { name: "Run request" }));
  expect(screen.getByRole("alert")).toHaveTextContent("JSON object");
  expect(fetch).not.toHaveBeenCalled();
});
/** 目的：验证任务查询可切换 GET、失败与双语原文保留；前置中文组件及网络失败边界，GET 不带正文，语言切换不改变输入，清理替身。 */
it("supports GET queries and translated network failures without changing input", async () => {
  const fetch = vi.fn().mockRejectedValue(new Error("offline"));
  vi.stubGlobal("fetch", fetch);
  setup("zh-CN");
  fireEvent.change(screen.getByLabelText("XHub API 密钥"), { target: { value: "key" } });
  fireEvent.change(screen.getByLabelText("请求方法"), { target: { value: "GET" } });
  fireEvent.change(screen.getByLabelText("请求路径"), { target: { value: "/v1/videos/user-original-id" } });
  fireEvent.click(screen.getByRole("button", { name: "运行请求" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("请求失败或超时");
  expect(fetch).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ method: "GET", body: undefined }));
  fireEvent.click(screen.getByRole("button", { name: "English" }));
  expect(screen.getByLabelText("Request path")).toHaveValue("/v1/videos/user-original-id");
  await waitFor(() => expect(screen.getByRole("button", { name: "Run request" })).toBeEnabled());
});
/** 目的：验证文件上传和 HTTP 错误直接显示；前置文件与 401 响应边界，multipart 使用浏览器边界且保留返回正文，测试后清理替身。 */
it("uploads multipart files and exposes gateway error responses", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(
      new Response('{"error":"invalid key"}', { status: 401, headers: { "content-type": "application/json" } }),
    );
  vi.stubGlobal("fetch", fetch);
  setup();
  fireEvent.change(screen.getByLabelText("XHub API key"), { target: { value: "invalid" } });
  fireEvent.click(screen.getByLabelText("Use multipart file upload"));
  fireEvent.change(screen.getByLabelText("Upload file"), { target: { files: [new File(["audio"], "clip.wav")] } });
  fireEvent.click(screen.getByRole("button", { name: "Run request" }));
  expect(await screen.findByRole("status")).toHaveTextContent("HTTP 401");
  const options = fetch.mock.calls[0][1];
  expect(options.body).toBeInstanceOf(FormData);
  expect(options.body.get("file").name).toBe("clip.wav");
  expect(options.headers["Content-Type"]).toBeUndefined();
});

/** 目的：公开目录无密钥可执行但推理不可匿名；前置真实组件与成功目录响应，验证无 Authorization 及拒绝路径，不写业务数据。 */
it("allows anonymous root model discovery only", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response('{"data":[]}', { headers: { "content-type": "application/json" } }));
  vi.stubGlobal("fetch", fetch);
  setup();
  fireEvent.change(screen.getByLabelText("HTTP method"), { target: { value: "GET" } });
  fireEvent.change(screen.getByLabelText("Request path"), { target: { value: "/models" } });
  fireEvent.click(screen.getByRole("button", { name: "Run request" }));
  expect(await screen.findByRole("status")).toHaveTextContent("HTTP 200");
  expect(fetch.mock.calls[0][1].headers.Authorization).toBeUndefined();
  fireEvent.change(screen.getByLabelText("Request path"), { target: { value: "/v1/models" } });
  fireEvent.click(screen.getByRole("button", { name: "Run request" }));
  expect(screen.getByRole("alert")).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(1);
});
