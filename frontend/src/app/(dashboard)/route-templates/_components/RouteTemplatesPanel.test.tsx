import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RouteTemplatesPanel from "./RouteTemplatesPanel";
import { createRouteTemplateCall, getRouteTemplatesCall, updateRouteTemplateCall } from "@/components/networking";

const document = (timeout = 60) => ({
  model_routes: [],
  retry_policy: { max_attempts: 2, timeout_seconds: timeout, failure_threshold: 3, cooldown_seconds: 0 },
});

vi.mock("@/components/networking", () => ({
  apiClient: { get: vi.fn(async () => ({ endpoint_types: [] })), post: vi.fn() },
  getRouteTemplatesCall: vi.fn(async () => [
    { id: "first", name: "Fast", body: document(20), writable: true, used_by: 1 },
    { id: "second", name: "Shared", body: document(), writable: false, used_by: 0 },
  ]),
  modelInfoCall: vi.fn(async () => ({ data: [], total_pages: 1 })),
  userAvailableModelsCall: vi.fn(async () => ({ data: [] })),
  createRouteTemplateCall: vi.fn(async () => ({})),
  updateRouteTemplateCall: vi.fn(async () => ({})),
  getRouteTemplateUsageCall: vi.fn(async () => ({ data: [] })),
  deleteRouteTemplateCall: vi.fn(async () => ({})),
}));

/** 渲染路由模板页面并返回用户驱动器；查询缓存随组件卸载清理。 */
function setup() {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <RouteTemplatesPanel accessToken="fixture" />
    </QueryClientProvider>,
  );
  return user;
}

/** 打开新模板编辑器；等待模板库加载完成后返回编辑区域。 */
async function openCreate(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("Fast");
  await user.click(screen.getByRole("button", { name: "New template" }));
  return screen.getByRole("region", { name: "New template" });
}

describe("route template workflow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getRouteTemplatesCall).mockResolvedValue([
      { id: "first", name: "Fast", body: document(20), writable: true, used_by: 1 },
      { id: "second", name: "Shared", body: document(), writable: false, used_by: 0 },
    ]);
  });

  /** 验证新模板使用固定新骨架，名称去空格，保存内容不来自平台配置。 */
  it("creates from the builtin document", async () => {
    const user = setup();
    const editor = await openCreate(user);
    await user.click(within(editor).getByRole("tab", { name: "负载均衡" }));
    expect(within(editor).getByLabelText("Timeout (seconds)")).toHaveValue(60);
    await user.type(within(editor).getByLabelText("Name"), "  Custom  ");
    fireEvent.change(within(editor).getByLabelText("Timeout (seconds)"), { target: { value: "30" } });
    await user.click(within(editor).getByRole("button", { name: "保存模板" }));
    await waitFor(() =>
      expect(createRouteTemplateCall).toHaveBeenCalledWith(
        "fixture",
        expect.objectContaining({
          name: "Custom",
          body: {
            routing_groups: [], fallbacks: [], context_window_fallbacks: [], content_policy_fallbacks: [],
            model_routes: [],
            retry_policy: { max_attempts: 1, timeout_seconds: 30, failure_threshold: 3, cooldown_seconds: 60 },
          },
        }),
      ),
    );
  });

  /** 验证页面完全移除平台默认编辑区，只保留命名模板。 */
  it("does not render a platform default", async () => {
    setup();
    await screen.findByText("Fast");
    expect(screen.queryByText("Platform default")).not.toBeInTheDocument();
    expect(screen.queryByText(/设置模板默认策略/)).not.toBeInTheDocument();
  });

  /** 验证复制产生独立草稿，取消不会调用创建或更新接口。 */
  it("copies as a draft and cancels without saving", async () => {
    const user = setup();
    await screen.findByText("Fast");
    await user.click(screen.getByRole("button", { name: "Actions for Fast" }));
    await user.click(await screen.findByRole("menuitem", { name: "Copy" }));
    const editor = screen.getByRole("region", { name: "New template" });
    expect(within(editor).getByLabelText("Name")).toHaveValue("Fast copy");
    await user.click(within(editor).getByRole("tab", { name: "负载均衡" }));
    expect(within(editor).getByLabelText("Timeout (seconds)")).toHaveValue(20);
    await user.click(within(editor).getByRole("button", { name: "Cancel" }));
    expect(createRouteTemplateCall).not.toHaveBeenCalled();
    expect(updateRouteTemplateCall).not.toHaveBeenCalled();
  });

  /** 前置新模板；验证所有编辑来自表单、JSON只读且反映最新值，保存同一文档；RTL卸载清理。 */
  it("synchronizes editable JSON with forms and saves the document", async () => {
    const user = setup();
    const editor = await openCreate(user);
    await user.type(within(editor).getByLabelText("Name"), "Form template");
    await user.click(within(editor).getByRole("tab", { name: "负载均衡" }));
    fireEvent.change(within(editor).getByLabelText("Timeout (seconds)"), { target: { value: "90" } });
    await user.click(within(editor).getByRole("tab", { name: "JSON" }));
    const json = within(editor).getByRole("textbox", { name: "JSON" });
    expect(json).not.toHaveAttribute("readonly");
    const next = JSON.parse((json as HTMLTextAreaElement).value);
    expect(next.retry_policy.timeout_seconds).toBe(90);
    await user.click(within(editor).getByRole("button", { name: "保存模板" }));
    await waitFor(() =>
      expect(createRouteTemplateCall).toHaveBeenCalledWith("fixture", expect.objectContaining({ body: next })),
    );
  });

  /** 前置合法草稿和旧格式文件；验证导入校验阻止保存，草稿不被非法JSON覆盖，无后台写入。 */
  it("rejects unsupported imported JSON", async () => {
    const user = setup();
    const editor = await openCreate(user);
    await user.type(within(editor).getByLabelText("Name"), "Invalid import");
    await user.click(within(editor).getByRole("tab", { name: "JSON" }));
    const file = new File(['{"num_retries":2}'], "legacy.json", { type: "application/json" });
    Object.defineProperty(file, "text", { value: async () => '{"num_retries":2}' });
    fireEvent.change(within(editor).getByLabelText("Upload JSON"), { target: { files: [file] } });
    expect(await within(editor).findByRole("alert")).toBeVisible();
    expect(within(editor).getByRole("button", { name: "保存模板" })).toBeDisabled();
    expect(createRouteTemplateCall).not.toHaveBeenCalled();
  });

  /** 前置合法草稿和读取失败的文件；验证导入显示错误且阻止提交，组件卸载清理状态，无后台写入。 */
  it("blocks saving when an imported file cannot be read", async () => {
    const user = setup();
    const editor = await openCreate(user);
    await user.type(within(editor).getByLabelText("Name"), "Unreadable");
    await user.click(within(editor).getByRole("tab", { name: "JSON" }));
    const file = new File(["invalid"], "broken.json", { type: "application/json" });
    Object.defineProperty(file, "text", {
      value: async () => {
        throw new Error("read failure");
      },
    });
    const input = within(editor).getByLabelText("Upload JSON");
    fireEvent.change(input, { target: { files: [file] } });
    expect(await within(editor).findByRole("alert")).toHaveTextContent("无法读取 JSON 文件");
    expect(within(editor).getByRole("button", { name: "保存模板" })).toBeDisabled();
    expect(createRouteTemplateCall).not.toHaveBeenCalled();
  });

  /** 验证模板名称和正文作为一次更新提交，未引入平台文档合并。 */
  it("renames and edits an existing template", async () => {
    const user = setup();
    await screen.findByText("Fast");
    await user.click(within(screen.getByRole("row", { name: /Fast/ })).getByRole("button", { name: "Edit" }));
    const editor = screen.getByRole("region", { name: "Edit" });
    await user.clear(within(editor).getByLabelText("Name"));
    await user.type(within(editor).getByLabelText("Name"), "Renamed");
    await user.click(within(editor).getByRole("button", { name: "保存模板" }));
    await waitFor(() =>
      expect(updateRouteTemplateCall).toHaveBeenCalledWith("fixture", "first", { name: "Renamed", body: document(20) }),
    );
  });

  /** 验证完整帮助分区、可导入示例和待接入边界；组件卸载清理展示状态。 */
  it("opens complete JSON documentation without changing drafts", async () => {
    const user = setup();
    await screen.findByText("Fast");
    await user.click(screen.getByRole("button", { name: "完整 JSON 配置指南" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByRole("cell", { name: "routing_groups" })).toBeInTheDocument();
    await user.click(within(dialog).getByRole("tab", { name: "可导入示例" }));
    expect(within(dialog).getByRole("region", { name: "完整模板示例" })).toHaveTextContent(
      '"routing_strategy": "cost-based-routing"',
    );
    await user.click(within(dialog).getByRole("tab", { name: "LiteLLM 对照" }));
    expect(within(dialog).getByText(/负载均衡/, {selector: "td"})).toBeInTheDocument();
    expect(createRouteTemplateCall).not.toHaveBeenCalled();
  });

  /** 验证新模板具有三个表单与JSON，无默认策略和预览侧栏；取消不写后台，卸载清理。 */
  it("contains all three routing sections in each template", async () => {
    const user = setup();
    const editor = await openCreate(user);
    expect(within(editor).getAllByRole("tab").map(tab => tab.textContent)).toEqual(["负载均衡", "路由组", "故障转移", "JSON"]);
    expect(within(editor).queryByLabelText("默认路由策略")).not.toBeInTheDocument();
    expect(within(editor).queryByRole("complementary")).not.toBeInTheDocument();
    await user.click(within(editor).getByRole("tab", {name: "故障转移"}));
    expect(within(editor).getByRole("button", {name: "添加故障转移"})).toBeInTheDocument();
    await user.click(within(editor).getByRole("button", {name: "Cancel"}));
    expect(createRouteTemplateCall).not.toHaveBeenCalled();
  });

  /** 验证模板库加载失败时禁用创建，重试成功后恢复。 */
  it("offers a retry when the template library fails", async () => {
    vi.mocked(getRouteTemplatesCall).mockRejectedValueOnce(new Error("unavailable"));
    const user = setup();
    await screen.findByText("Failed to load the templates");
    expect(screen.getByRole("button", { name: "New template" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Reload" }));
    await screen.findByText("Fast");
    expect(screen.getByRole("button", { name: "New template" })).toBeEnabled();
  });
});
