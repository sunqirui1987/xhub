import { act, render, screen, waitFor, within } from "@testing-library/react";
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
 * 参数：初始草稿和目录 ID；返回：测试用表单。只有端点目录的网络边界使用假数据。
 */
function EndpointForm({ defaultValues = {}, catalogId = "" }: { defaultValues?: MountedFormValues; catalogId?: string }) {
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
          catalogId={catalogId}
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
  /** 前置对话目录和真实表单；验证默认折叠、键盘展开、无路径项、直通勾选及反复折叠不丢声明；自动卸载清理。 */
  it("collapses endpoint cards by default and preserves optional bypass selections", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      endpoint_types: [
        {id:"chat",label:"OpenAI · Chat Completions",kind:"adapted",protocol:"openai-chat",paths:["/v1/chat/completions"]},
        {id:"gemini",label:"Gemini · Generate Content",kind:"adapted",protocol:"gemini",paths:["/v1beta/models/{model}:generateContent","/v1beta/models/{model}:streamGenerateContent"]},
        {id:"messages",label:"Anthropic · Messages",kind:"adapted",protocol:"anthropic-messages"},
        {id:"bypass:openai-responses",label:"Bypass · OpenAI Responses",kind:"bypass",protocol:"openai-responses",paths:["/bypass/openai/v1/responses"]},
      ],transports:dialogueTransports, models:{},
    });
    const user = userEvent.setup();
    render(<EndpointForm defaultValues={{model:"group/model",transport:"bypass_openai_responses",endpoint_types:["chat","gemini","messages"]}}/>);
    const region = await screen.findByRole("region", {name:"XHub 对外接口"});
    const toggle = within(region).getByRole("button", {name:"XHub 对外接口"});
    expect(toggle).toHaveAttribute("type", "button");
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(within(region).queryByRole("list")).not.toBeInTheDocument();
    expect(within(region).queryByRole("checkbox")).not.toBeInTheDocument();
    expect(within(region).getByText("/v1/chat/completions")).not.toBeVisible();
    toggle.focus();
    await user.keyboard("{Enter}");
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(within(region).getAllByRole("listitem")).toHaveLength(3);
    const gemini = within(region).getByRole("listitem", {name:"Gemini · Generate Content"});
    expect(within(gemini).getByText("/v1beta/models/{model}:generateContent").tagName).toBe("CODE");
    expect(within(gemini).getByText("/v1beta/models/{model}:streamGenerateContent").tagName).toBe("CODE");
    const bypass = within(region).getByRole("checkbox", {name:"Bypass · OpenAI Responses"});
    expect(bypass).not.toBeChecked();
    await user.click(bypass);
    expect(bypass).toBeChecked();
    expect(screen.getByLabelText("部署声明")).toHaveTextContent("bypass:openai-responses");
    await user.click(toggle);
    expect(bypass).not.toBeVisible();
    expect(screen.getByLabelText("部署声明")).toHaveTextContent("bypass:openai-responses");
    await user.click(toggle);
    expect(bypass).toBeChecked();
    await user.click(bypass);
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":["chat","gemini","messages"]');
  });
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
        endpoint_types: [{ id: "chat", category: "openai", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }],
        transports: dialogueTransports,
        models: { "test_supplier/fal-ai/kling-video/v2.5/text-to-video": "bypass_openai_chat" },
      }),
    );
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":["chat"]'));
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_chat"');
    await user.click(screen.getByRole("combobox", { name: "上游接口协议" }));
    await user.click(await screen.findByRole("option", { name: "OpenAI · Chat Completions" }));
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
    expect(screen.queryByRole("button", {name:"XHub 对外接口"})).not.toBeInTheDocument();
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_chat"');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("retains the visible saved chat endpoint when the directory arrives late for an unregistered model", async () => {
    let resolveCatalog!: (payload: unknown) => void;
    vi.mocked(apiClient.get).mockReturnValue(new Promise(resolve => { resolveCatalog = resolve; }));
    const user = userEvent.setup();
    render(<EndpointForm defaultValues={{ model: "test_supplier/glm-4.5", endpoint_types: ["chat"], transport: "bypass_openai_chat" }} />);
    await user.click(screen.getByRole("button", { name: "编辑描述" }));
    await act(async () => resolveCatalog({ endpoint_types: [{ id: "chat", category: "openai", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }], transports: dialogueTransports, models: {} }));
    expect(screen.getByRole("combobox", { name: "上游接口协议" })).toHaveTextContent("OpenAI · Chat Completions");
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_chat"');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("leaves unknown models unselected and preserves a manual endpoint on parent updates", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      endpoint_types: [{ id: "chat", category: "openai", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }],
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
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":[]');
    await user.click(await screen.findByRole("option", { name: "OpenAI · Responses", exact: true }));
    await waitFor(() => expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":["chat"]'));
    await user.click(screen.getByRole("button", { name: "编辑描述" }));
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"bypass_openai_responses"');
  });

  // 前置真实表单与确定性目录；验证加载、失败和声明联动，测试库自动卸载，无持久数据。
  it("pins fixed Fal models to their own transport and clears unsupported selections", async () => {
    vi.mocked(apiClient.get).mockResolvedValue({
      endpoint_types: [{ id: "fal:queue", category: "bypass", label: "Fal 视频", kind: "bypass", protocol: "fal", family: "video" }],
      transports: [
        {
          id: "kling", protocol: "fal", label: "Kling",
          endpoint_id: "fal:queue", category: "bypass",
          providers: ["test_supplier"],
          strip_prefix: "test_supplier",
          actions: [{ name: "create", model: "fal-ai/kling-video/v2.5/text-to-video" }],
        },
        {
          id: "vidu", protocol: "fal", label: "Vidu",
          endpoint_id: "fal:queue", category: "bypass",
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
    vi.mocked(apiClient.get).mockResolvedValue({ endpoint_types: [{ id: "chat", category: "openai", label: "聊天协议", kind: "adapted", protocol: "openai-chat", family: "chat" }], transports: [] });
    render(<EndpointForm defaultValues={{ endpoint_types: ["chat"], transport: "bypass_openai_chat" }} />);
    await waitFor(() => expect(screen.getByRole("combobox", { name: "上游接口协议" })).toBeDisabled());
    expect(screen.getByText(/目录未提供可用的上游接口协议，暂时无法保存该模型/)).toBeVisible();
    expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":""');
  });


});

/** 前置目录声明、真实表单和两个固定 Fal 实现；验证专用型号自动选择、通用菜单保留和型号切换；自动卸载清理。 */
it("defaults dedicated models while keeping connection-compatible choices", async () => {
 vi.mocked(apiClient.get).mockResolvedValue({
  endpoint_types:[{id:"fal:queue",label:"Fal",kind:"bypass",protocol:"fal",family:"video"}],
  transports:[
   {id:"kling",catalog_id:"qiniu",label:"Kling",protocol:"fal",endpoint_id:"fal:queue",providers:["test_supplier"]},
   {id:"vidu",catalog_id:"qiniu",label:"Vidu",protocol:"fal",endpoint_id:"fal:queue",providers:["test_supplier"]},
   ...dialogueTransports,
  ],catalogs:{qiniu:{"test_supplier/fal-ai/kling-video/v2.5/text-to-video":["kling"],"test_supplier/fal-ai/vidu/q3/text-to-video":["vidu"]}},
 });
 const user=userEvent.setup();render(<EndpointForm catalogId="qiniu"/>);
 await waitFor(()=>expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"kling"'));
 await user.click(screen.getByRole("combobox",{name:"上游接口协议"}));
 const listbox=await screen.findByRole("listbox");
 expect(within(listbox).getByRole("option",{name:"OpenAI · Chat Completions",exact:true})).toBeVisible();
 await user.click(screen.getByRole("option",{name:"FAL · Kling",exact:true}));
 await user.click(screen.getByRole("button",{name:"切换 Vidu"}));
 await waitFor(()=>expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"vidu"'));
 await user.click(screen.getByRole("button",{name:"切换未知模型"}));
 await waitFor(()=>expect(screen.getByRole("combobox",{name:"上游接口协议"})).toBeEnabled());
 expect(screen.getByLabelText("部署声明")).toHaveTextContent('"endpoint_types":[]');
 expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":""');
});

/** 前置未知目录模型或不兼容连接；验证具体不可保存原因且不显示无关 FAL 提示；测试库卸载，无持久数据清理。 */
it("explains unsupported connection types without unrelated FAL help", async () => {
 vi.mocked(apiClient.get).mockResolvedValue({endpoint_types:[],transports:[{id:"fal",protocol:"fal",label:"Dreamina",endpoint_id:"fal:queue",catalog_id:"qiniu",providers:["custom"]}],catalogs:{qiniu:{"registered/model":["fal"]}}});
 const view=render(<EndpointForm catalogId="qiniu" defaultValues={{model:"unknown/model"}}/>);
 expect(await screen.findByText(/目录未提供可用的上游接口协议/)).toBeVisible();
 expect(screen.queryByText(/上游模型需填写 FAL 任务路径/)).not.toBeInTheDocument();
 view.unmount();
 render(<EndpointForm catalogId="qiniu" defaultValues={{model:"registered/model"}}/>);
 expect(await screen.findByText(/当前连接类型 test_supplier 不支持该模型登记的上游协议/)).toBeVisible();
});

/** 前置未注册中转目录及已实现的 FAL 型号；验证协议可选择、错误型号显示说明且不借用价格能力；卸载清理，无持久数据。 */
it("allows explicit FAL task paths on an unregistered relay catalog", async () => {
 vi.mocked(apiClient.get).mockResolvedValue({endpoint_types:[],transports:[{id:"fal",protocol:"fal",model_group:"Dreamina",label:"Dreamina",endpoint_id:"fal:queue",providers:["test_supplier"],actions:[{name:"create",public_path:"/queue/byteplus/seedance-2.0/text-to-video",model:"byteplus/seedance-2.0/text-to-video"}]}],catalogs:{qiniu:{"byteplus/seedance-2.0/text-to-video":["fal"]}}});
 const view=render(<EndpointForm catalogId="fennoai" defaultValues={{model:"byteplus/seedance-2.0/text-to-video",transport:"fal"}}/>);
 await waitFor(()=>expect(screen.getByRole("combobox",{name:"上游接口协议"})).toHaveTextContent("FAL · Dreamina"));
 expect(screen.getByRole("combobox",{name:"上游接口协议"})).toBeEnabled();
 view.unmount();
 render(<EndpointForm catalogId="fennoai" defaultValues={{model:"unknown/video"}}/>);
 expect(await screen.findByText(/请选择上游实际支持的调用协议/)).toBeVisible();
 expect(screen.getByRole("combobox",{name:"上游接口协议"})).toBeEnabled();
});

/** 前置真实表单、七牛目录与常规协议；普通型号默认 OpenAI 且无无关 FAL 说明，折叠时仍可切换 Responses，卸载清理。 */
it("defaults an ordinary listed model to OpenAI even with a qiniu catalog", async () => {
 vi.mocked(apiClient.get).mockResolvedValue({endpoint_types:[{id:"chat",label:"Chat",kind:"adapted",protocol:"openai-chat"}],transports:dialogueTransports,catalogs:{qiniu:{video:["fal"]}}});
 render(<OpenAIEndpointForm/>);
 await waitFor(()=>expect(screen.getByRole("combobox",{name:"上游接口协议"})).toHaveTextContent("OpenAI · Chat Completions"));
 expect(screen.getByRole("combobox",{name:"上游接口协议"})).toBeEnabled();
 expect(screen.queryByText(/上游模型需填写 FAL 任务路径/)).not.toBeInTheDocument();
 expect(screen.getByRole("button",{name:"XHub 对外接口"})).toHaveAttribute("aria-expanded","false");
 const user=userEvent.setup();await user.click(screen.getByRole("combobox",{name:"上游接口协议"}));
 await user.click(await screen.findByRole("option",{name:"OpenAI · Responses",exact:true}));
 expect(screen.getByRole("combobox",{name:"上游接口协议"})).toHaveTextContent("OpenAI · Responses");
});
/** OpenAIEndpointForm 挂载 OpenAI 连接及普通模型，返回真实表单；调用本用例，无网络和持久化副作用。 */
function OpenAIEndpointForm() {
 const form=useForm<MountedFormValues>({defaultValues:{model:"gpt-5.6-sol"}});const registry=useMountRegistry();
 return <FormProvider {...form}><MountedFormProvider value={{control:form.control,registry}}><EndpointTypeField selectedProvider="openai" catalogId="qiniu"/></MountedFormProvider></FormProvider>;
}

/** 前置 FAL 专用实现与不支持的型号；用户选择后显示简短路径提示，折叠区外直接解释限制，后台拒绝错误路径；卸载清理。 */
it("lets users select an implemented FAL protocol and explains an unsupported model", async () => {
 vi.mocked(apiClient.get).mockResolvedValue({endpoint_types:[],transports:[{id:"fal",protocol:"fal",label:"Dreamina",model_group:"Dreamina",endpoint_id:"fal:queue",catalog_id:"qiniu",providers:["test_supplier"],actions:[{name:"create",model:"byteplus/seedance-2.0/text-to-video"}]}],catalogs:{qiniu:{"byteplus/seedance-2.0/text-to-video":["fal"]}}});
 render(<EndpointForm catalogId="qiniu" defaultValues={{model:"unknown/model"}}/>);
 const user=userEvent.setup();await waitFor(()=>expect(screen.getByRole("combobox")).toBeEnabled());
 await user.click(screen.getByRole("combobox"));await user.click(await screen.findByRole("option",{name:"FAL · Dreamina",exact:true}));
 expect(await screen.findByRole("alert")).toHaveTextContent("尚未实现当前模型或目录的调用");
 expect(screen.getByText(/上游模型需填写 FAL 任务路径/)).toBeVisible();
 expect(screen.getByRole("button",{name:"XHub 对外接口"})).toHaveAttribute("aria-expanded","false");
 expect(screen.getByLabelText("部署声明")).toHaveTextContent('"transport":"fal"');
});
