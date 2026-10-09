import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { FormProvider, useForm, useWatch } from "react-hook-form";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MountedFormProvider, useMountRegistry, type MountedFormValues } from "../common_components/MountedFormField";
import EndpointTypeField from "./EndpointTypeField";
import { apiClient } from "../networking";

vi.mock("../networking", () => ({ apiClient: { get: vi.fn() } }));
afterEach(() => vi.clearAllMocks());
const dialogueTransports = [
  { id: "bypass_openai_chat", label: "内部 Chat 传输", kind: "bypass", endpoint_id: "bypass:openai-chat", protocol: "openai-chat" },
  { id: "bypass_openai_responses", label: "内部 Responses 传输", kind: "bypass", endpoint_id: "bypass:openai-responses", protocol: "openai-responses" },
  { id: "bypass_anthropic_messages", label: "内部 Messages 传输", kind: "bypass", endpoint_id: "bypass:anthropic-messages", protocol: "anthropic-messages" },
];

/** EndpointForm 挂载真实端点字段和表单，暴露提交契约以验证模型切换后的联动。
 * 参数：无；返回：测试用表单。只有端点目录的网络边界使用假数据。
 */
function EndpointForm({ defaultValues = {} }: { defaultValues?: MountedFormValues }) {
  const form = useForm<MountedFormValues>({
    defaultValues: { model: "test_supplier/fal-ai/kling-video/v2.5/text-to-video", ...defaultValues },
  });
  const registry = useMountRegistry();
  const values = useWatch({ control: form.control });
  return (
    <FormProvider {...form}>
      <MountedFormProvider value={{ control: form.control, registry }}>
        <EndpointTypeField
          selectedProvider="test_supplier"
          modelCostMap={{ "test_supplier/unknown-model": { endpoint_id: "chat" } }}
        />
        <button onClick={() => form.setValue("model", "test_supplier/fal-ai/vidu/q3/text-to-video")}>切换 Vidu</button>
        <button onClick={() => form.setValue("model", "test_supplier/unknown-model")}>切换未知模型</button>
        <button onClick={() => form.setValue("description", "updated")}>编辑描述</button>
        <output aria-label="部署声明">{JSON.stringify(values)}</output>
      </MountedFormProvider>
    </FormProvider>
  );
}

describe("model scoped endpoint transports", () => {
  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("settles while the endpoint catalog is loading and applies defaults when it arrives", async () => {
    let resolveCatalog!: (payload: unknown) => void;
    vi.mocked(apiClient.get).mockReturnValue(
      new Promise((resolve) => {
        resolveCatalog = resolve;
      }),
    );
    const user = userEvent.setup();
    render(
      <StrictMode>
        <EndpointForm />
      </StrictMode>,
    );
    expect(screen.getByRole("combobox")).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "编辑描述" }));
    expect(screen.getByRole("combobox")).toHaveTextContent("正在加载端点目录");
    await act(async () =>
      resolveCatalog({
        endpoint_types: [{ id: "chat", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }],
        transports: dialogueTransports,
        models: { "test_supplier/fal-ai/kling-video/v2.5/text-to-video": "chat" },
      }),
    );
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":["chat"]'));
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":""');
    await user.click(screen.getByRole("combobox", { name: "上游接口协议" }));
    await user.click(screen.getByRole("option", { name: "Chat Completions" }));
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_chat"');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("clears stale declarations with a successfully loaded empty catalog without looping", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ endpoint_types: [], transports: [], models: {} });
    const user = userEvent.setup();
    render(
      <StrictMode>
        <EndpointForm defaultValues={{ endpoint_types: ["chat"], transport: "bypass_openai_chat" }} />
      </StrictMode>,
    );
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":[]'));
    await user.click(screen.getByRole("button", { name: "编辑描述" }));
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":""');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("preserves saved endpoint declarations while loading and after a directory failure", async () => {
    let rejectCatalog!: (error: Error) => void;
    vi.mocked(apiClient.get).mockReturnValue(new Promise((_resolve, reject) => { rejectCatalog = reject; }));
    render(<EndpointForm defaultValues={{ model: "test_supplier/glm-4.5", endpoint_types: ["chat"], transport: "bypass_openai_chat" }} />);
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":["chat"]');
    await act(async () => rejectCatalog(new Error("offline")));
    expect(await screen.findByRole("alert")).toHaveTextContent("已保存的端点声明已保留");
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_chat"');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("retains the visible saved chat endpoint when the directory arrives late for an unregistered model", async () => {
    let resolveCatalog!: (payload: unknown) => void;
    vi.mocked(apiClient.get).mockReturnValue(new Promise(resolve => { resolveCatalog = resolve; }));
    const user = userEvent.setup();
    render(<EndpointForm defaultValues={{ model: "test_supplier/glm-4.5", endpoint_types: ["chat"], transport: "bypass_openai_chat" }} />);
    await user.click(screen.getByRole("button", { name: "编辑描述" }));
    await act(async () => resolveCatalog({ endpoint_types: [{ id: "chat", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }], transports: dialogueTransports, models: {} }));
    expect(screen.getByRole("combobox", { name: "Endpoint type" })).toHaveTextContent("聊天协议");
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_chat"');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("leaves unknown models unselected and preserves a manual endpoint on parent updates", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      endpoint_types: [{ id: "chat", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }],
      transports: dialogueTransports,
      models: {},
    });
    const user = userEvent.setup();
    render(
      <StrictMode>
        <EndpointForm defaultValues={{ model: "test_supplier/unknown-model" }} />
      </StrictMode>,
    );
    await user.click(screen.getByRole("combobox"));
    const option = await screen.findByRole("option", { name: "聊天协议" });
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":[]');
    await user.click(option);
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":""');
    await user.click(screen.getByRole("combobox", { name: "上游接口协议" }));
    await user.click(await screen.findByRole("option", { name: "Responses", exact: true }));
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":["chat"]'));
    await user.click(screen.getByRole("button", { name: "编辑描述" }));
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_responses"');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("pins fixed Fal models to their own transport and clears unsupported selections", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      endpoint_types: [{ id: "bypass:fal-video", label: "Fal 视频", kind: "bypass", protocol: "fal", family: "video" }],
      transports: [
        {
          id: "kling",
          endpoint_id: "bypass:fal-video",
          providers: ["test_supplier"],
          strip_prefix: "test_supplier",
          actions: [{ name: "create", model: "fal-ai/kling-video/v2.5/text-to-video" }],
        },
        {
          id: "vidu",
          endpoint_id: "bypass:fal-video",
          providers: ["test_supplier"],
          strip_prefix: "test_supplier",
          actions: [{ name: "create", model: "fal-ai/vidu/q3/text-to-video" }],
        },
      ],
      models: { "test_supplier/fal-ai/kling-video/v2.5/text-to-video": "kling", "test_supplier/fal-ai/vidu/q3/text-to-video": "vidu" },
    });
    const user = userEvent.setup();
    render(<EndpointForm />);
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"kling"'));
    await user.click(screen.getByRole("button", { name: "切换 Vidu" }));
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"vidu"'));
    await user.click(screen.getByRole("button", { name: "切换未知模型" }));
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":[]'));
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":""');
  });
  /** 前置空执行目录和已声明对话入口；验证不能隐式推断协议，显示不可保存原因；自动卸载清理。 */
  it("reports unavailable upstream profiles without guessing a transport", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ endpoint_types: [{ id: "chat", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }], transports: [] });
    render(<EndpointForm defaultValues={{ endpoint_types: ["chat"], transport: "bypass_openai_chat" }} />);
    await waitFor(() => expect(screen.getByRole("combobox", { name: "上游接口协议" })).toBeDisabled());
    expect(screen.getByText("目录未提供可用的上游接口协议，暂时无法保存该模型。")).toBeVisible();
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":""');
  });


});
