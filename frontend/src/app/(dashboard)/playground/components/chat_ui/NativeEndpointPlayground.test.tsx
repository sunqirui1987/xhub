import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import NativeEndpointPlayground from "./NativeEndpointPlayground";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";
import { act } from "@testing-library/react";

vi.mock("@/components/networking", () => ({ getProxyBaseUrl: () => "http://localhost:4100" }));
const endpoint: ModelEndpoint = {
  endpoint_id: "fal:queue",
  kind: "bypass",
  transport: "qiniu_fal_dreamina_20",
  protocol: "fal",
  family: "video",
  method: "POST",
  path: "/queue/create",
  actions: [
    { name: "get", method: "GET", public_path: "/queue/requests/{request_id}" },
    { name: "status", method: "GET", public_path: "/queue/requests/{request_id}/status" },
  ],
};

/** 每个用例结束恢复全局 fetch；参数/返回无，组件由测试环境卸载，无外部数据。 */
afterEach(() => vi.unstubAllGlobals());

/** 前置挂起的本地 fetch；验证停止会中止请求并恢复提交入口，结束恢复全局状态，无外部任务。 */
it("停止挂起请求后显示明确状态并可再次提交", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(
      (_url, options) =>
        new Promise((_resolve, reject) => {
          options.signal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")));
        }),
    ),
  );
  render(<NativeEndpointPlayground endpoint={endpoint} model="demo" apiKey="session" />);
  fireEvent.click(screen.getByRole("button", { name: "提交请求" }));
  fireEvent.click(screen.getByRole("button", { name: "停止" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("请求已停止");
  expect(screen.getByRole("button", { name: "提交请求" })).toBeEnabled();
});

/** 前置延迟响应夹具；验证换模型中止旧请求，迟到结果不污染新模型，无网络及持久数据。 */
it("切换模型忽略旧请求的迟到结果", async () => {
  let resolve!: (response: Response) => void;
  const fetchMock = vi.fn().mockImplementation(
    () =>
      new Promise<Response>((done) => {
        resolve = done;
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const view = render(<NativeEndpointPlayground endpoint={endpoint} model="demo" apiKey="session" />);
  fireEvent.click(screen.getByRole("button", { name: "提交请求" }));
  view.rerender(<NativeEndpointPlayground endpoint={endpoint} model="next" apiKey="session" />);
  expect(fetchMock.mock.calls[0][1].signal.aborted).toBe(true);
  await act(async () => resolve(new Response(JSON.stringify({ request_id: "old" }))));
  expect(screen.getByLabelText("任务 ID")).toHaveValue("");
  expect(screen.queryByLabelText("原生响应")).not.toBeInTheDocument();
});

/** 前置本地组件和模拟 fetch；验证空态、缺密钥及无任务边界，不发网络请求，无数据清理。 */
it("未提交时显示空态，无密钥或任务不可调用", () => {
  render(<NativeEndpointPlayground endpoint={endpoint} model="demo" apiKey="" />);
  expect(screen.getByText("提交请求后，结果显示在这里")).toBeVisible();
  expect(screen.queryByLabelText("原生响应")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "提交请求" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "查询状态" })).toBeDisabled();
});

/** 前置模拟合法任务返回；验证模型注入、ID 回填、中文动作及路径编码，结束恢复 fetch。 */
it("提交回填任务并通过明确动作查询结果", async () => {
  const fetchMock = vi
    .fn()
    .mockImplementation(async () => new Response(JSON.stringify({ request_id: "task/1", status: "COMPLETED" })));
  vi.stubGlobal("fetch", fetchMock);
  render(<NativeEndpointPlayground endpoint={endpoint} model="demo" apiKey="session" />);
  fireEvent.change(screen.getByLabelText("原生请求参数"), { target: { value: '{"prompt":"hello"}' } });
  fireEvent.click(screen.getByRole("button", { name: "提交请求" }));
  await waitFor(() => expect(screen.getByLabelText("任务 ID")).toHaveValue("task/1"));
  expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ model: "demo", prompt: "hello" });
  fireEvent.click(screen.getByRole("button", { name: "查询状态" }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  expect(fetchMock.mock.calls[1][0]).toBe("http://localhost:4100/queue/requests/task%2F1/status");
  expect(await screen.findByLabelText("原生响应")).toHaveTextContent("COMPLETED");
});

/** 前置使用查询参数的任务端点；验证空白 ID 禁用查询、粘贴 ID 去除首尾空格，结束恢复 fetch，无外部数据。 */
it("任务查询拒绝空白 ID 并规范粘贴的查询参数", async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: "COMPLETED" })));
  vi.stubGlobal("fetch", fetchMock);
  render(
    <NativeEndpointPlayground
      endpoint={{
        ...endpoint,
        actions: [{ name: "status", method: "GET", public_path: "/task/status", task_query: "task_id" }],
      }}
      model="demo"
      apiKey="session"
    />,
  );
  fireEvent.change(screen.getByLabelText("任务 ID"), { target: { value: "   " } });
  expect(screen.getByRole("button", { name: "查询状态" })).toBeDisabled();
  expect(fetchMock).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("任务 ID"), { target: { value: "  task/1  " } });
  fireEvent.click(screen.getByRole("button", { name: "查询状态" }));
  expect(await screen.findByLabelText("原生响应")).toHaveTextContent("COMPLETED");
  expect(new URL(fetchMock.mock.calls[0][0]).searchParams.get("task_id")).toBe("task/1");
});

/** 前置本地组件；验证非对象及非法 JSON 被可见错误拒绝，fetch 无调用，恢复全局状态。 */
it.each(["[]", "null", "{bad"])("非法请求 %s 显示错误且不调用上游", async (input) => {
  const fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  render(<NativeEndpointPlayground endpoint={endpoint} model="demo" apiKey="session" />);
  fireEvent.change(screen.getByLabelText("原生请求参数"), { target: { value: input } });
  fireEvent.click(screen.getByRole("button", { name: "提交请求" }));
  expect(await screen.findByRole("alert")).not.toHaveTextContent(/^$/);
  expect(fetchMock).not.toHaveBeenCalled();
});

/** 前置失败 HTTP 返回；验证错误有独立提示并可重试，换模型清理旧响应与任务，恢复 fetch。 */
it("上游错误可见，切换模型重置任务和参数", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("upstream unavailable", { status: 503 })));
  const view = render(<NativeEndpointPlayground endpoint={endpoint} model="demo" apiKey="session" />);
  fireEvent.click(screen.getByRole("button", { name: "提交请求" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("upstream unavailable");
  expect(screen.getByRole("button", { name: "提交请求" })).toBeEnabled();
  view.rerender(<NativeEndpointPlayground endpoint={endpoint} model="next" apiKey="session" />);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.getByLabelText("任务 ID")).toHaveValue("");
});

/** 前置 Google 原生绑定与离线 fetch；验证 contents 默认值及提交无正文 model，恢复 fetch 无外部数据。 */
it.each(["gemini", "vertex"])("%s 使用原厂正文与已绑定模型路径", async (protocol) => {
 const fetchMock=vi.fn().mockResolvedValue(new Response(JSON.stringify({candidates:[{content:{parts:[{text:"ok"}]}}]})));
 vi.stubGlobal("fetch",fetchMock);
 const path=(protocol==="gemini"?"/v1beta/models/":"/vertex/v1/models/")+"demo:generateContent";
 render(<NativeEndpointPlayground endpoint={{...endpoint, endpoint_id:protocol, protocol, path, actions:[]}} model="demo" apiKey="session"/>);
 expect(JSON.parse((screen.getByLabelText("原生请求参数") as HTMLTextAreaElement).value)).toHaveProperty("contents");
 fireEvent.change(screen.getByLabelText("原生请求参数"),{target:{value:JSON.stringify({contents:[{parts:[{text:"hello"}]}],generationConfig:{temperature:0.2}})}});
 fireEvent.click(screen.getByRole("button",{name:"提交请求"}));
 expect(await screen.findByLabelText("原生响应")).toHaveTextContent("ok");
 expect(fetchMock.mock.calls[0][0]).toBe("http://localhost:4100"+path);
 expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({contents:[{parts:[{text:"hello"}]}],generationConfig:{temperature:0.2}});
});
