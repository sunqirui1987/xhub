import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getProviderCreateMetadata, type ProviderCreateInfo } from "../networking";
import CredentialModal from "./CredentialModal";

vi.mock("../networking", async () => ({ ...await vi.importActual("../networking"), getProviderCreateMetadata: vi.fn() }));
const metadata: ProviderCreateInfo[] = [
  { provider: "OpenAI", provider_display_name: "OpenAI", litellm_provider: "openai", credential_fields: [
    { key: "api_base", label: "API Base", default_value: "https://api.openai.com/v1" },
    { key: "organization", label: "Organization" },
    { key: "api_key", label: "OpenAI API Key", field_type: "password", required: true },
  ] },
  { provider: "NewSupplier", provider_display_name: "New Supplier", litellm_provider: "new_supplier", credential_fields: [
    { key: "api_base", label: "API Base", default_value: "https://new.example/v1" },
    { key: "api_key", label: "Supplier API Key", field_type: "password", required: true },
  ] },
  { provider: "CHATGPT", provider_display_name: "ChatGPT", litellm_provider: "chatgpt", credential_fields: [] },
];
/** 创建独立 QueryClient 并挂载弹窗；props 覆盖模式与回调，返回 RTL 实例。各测试自动卸载且无外部请求。 */
function renderModal(props: Partial<React.ComponentProps<typeof CredentialModal>> = {}) {
  return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false,gcTime:0}}})}>
    <CredentialModal open mode="add" onCancel={vi.fn()} onSubmit={vi.fn()} {...props} />
  </QueryClientProvider>);
}
/** 通过可访问选择器选择 label；返回操作完成 Promise，供切换用例使用，只修改当前草稿。 */
async function choose(label: string) {
  const user = userEvent.setup();
  const input = await screen.findByRole("combobox", {name: "Provider type"});
  await waitFor(() => expect(input).toBeEnabled());
  await user.clear(input);
  await user.type(input, label);
  await user.click(await screen.findByRole("option", {name: new RegExp(label)}));
}
/** 填写新建必填值；返回 Promise，前置已选供应商，供保存用例调用，无持久化副作用。 */
async function fillSupplier() {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Provider name"), "test-provider");
  await user.type(screen.getByLabelText("Supplier API Key"), "test-secret");
}
beforeEach(() => { vi.mocked(getProviderCreateMetadata).mockReset().mockResolvedValue(metadata); });

describe("CredentialModal", () => {
  /** 验证新建先选类型且未显示默认 OpenAI；无前置数据，RTL 自动清理。 */
  it("starts without provider fields", async () => {
    renderModal();
    await waitFor(() => expect(screen.getByRole("combobox",{name:"Provider type"})).toBeEnabled());
    expect(screen.queryByLabelText("OpenAI API Key")).not.toBeInTheDocument();
    expect(screen.getByRole("button",{name:"Add model provider"})).toBeDisabled();
  });
  /** 验证后台新增而枚举未知的供应商可保存，未触碰默认地址也提交；独立客户端自动清理。 */
  it("submits a metadata-only supplier and its untouched default", async () => {
    const submit=vi.fn(); renderModal({onSubmit:submit});
    await choose("New Supplier"); await fillSupplier();
    await userEvent.click(screen.getByRole("button",{name:"Add model provider"}));
    await waitFor(() => expect(submit).toHaveBeenCalledWith({credential_name:"test-provider", custom_llm_provider:"new_supplier", provider_id:"NewSupplier", api_key:"test-secret", api_base:"https://new.example/v1"}));
  });
  /** 验证切换保留名称且清旧 key、组织和地址，前置 OpenAI 草稿；无写入，自动卸载清理。 */
  it("clears previous authentication on provider switch", async () => {
    const submit=vi.fn(); renderModal({onSubmit:submit}); await choose("OpenAI");
    const user=userEvent.setup();
    await user.type(screen.getByLabelText("Provider name"),"kept-name");
    await user.type(screen.getByLabelText("OpenAI API Key"),"old-secret");
    await user.type(screen.getByLabelText("Organization"),"old-org");
    await user.clear(screen.getByLabelText("API Base")); await user.type(screen.getByLabelText("API Base"),"https://old.example");
    await choose("New Supplier");
    expect(screen.getByLabelText("Provider name")).toHaveValue("kept-name");
    expect(screen.getByLabelText("Supplier API Key")).toHaveValue("");
    expect(screen.getByLabelText("API Base")).toHaveValue("https://new.example/v1");
    await user.type(screen.getByLabelText("Supplier API Key"),"new-secret");
    await user.click(screen.getByRole("button",{name:"Add model provider"}));
    await waitFor(()=>expect(submit).toHaveBeenCalled());
    expect(submit.mock.calls[0][0]).not.toHaveProperty("organization");
  });
  /** 验证必填失败不发送请求；前置空密钥，RTL 自动清理。 */
  it("validates required authentication", async () => {
    const submit=vi.fn(); renderModal({onSubmit:submit}); await choose("OpenAI");
    await userEvent.type(screen.getByLabelText("Provider name"),"no-key");
    await userEvent.click(screen.getByRole("button",{name:"Add model provider"}));
    expect(await screen.findByText("Required")).toBeVisible(); expect(submit).not.toHaveBeenCalled();
  });
  /** 验证旧 slug 编辑正确填值、锁名称与类型，避免后台合并保留不同协议的旧认证；只改草稿，自动清理。 */
  it("hydrates legacy slug credentials and locks provider identity", async () => {
    renderModal({mode:"edit",existingCredential:{credential_name:"legacy",credential_info:{custom_llm_provider:"openai"},credential_values:{api_key:"legacy-secret",api_base:"https://legacy.example"}}});
    expect(await screen.findByLabelText("OpenAI API Key")).toHaveValue("legacy-secret");
    expect(screen.getByLabelText("Provider name")).toBeDisabled();
    expect(screen.getByRole("combobox",{name:"Provider type"})).toBeDisabled();
  });
  /** 验证无法在弹窗完成授权的 ChatGPT 订阅账户不进入可创建目录；无外部账户，自动清理。 */
  it("omits ChatGPT Subscription from the provider picker", async () => {
    renderModal();
    const user = userEvent.setup();
    const input = await screen.findByRole("combobox", {name: "Provider type"});
    await waitFor(() => expect(input).toBeEnabled());
    await user.type(input, "ChatGPT");
    expect(screen.queryByRole("option", {name: /ChatGPT/})).not.toBeInTheDocument();
  });
  for (const kind of ["false", "throw"] as const) {
    /** 验证两种保存失败均保留完整草稿并允许重试；失败回调无写入，自动卸载清理。 */
    it('keeps draft after save failure: '+kind, async () => {
      const submit=kind==="false"?vi.fn().mockResolvedValue(false):vi.fn().mockRejectedValue(new Error("failed"));
      renderModal({onSubmit:submit}); await choose("New Supplier"); await fillSupplier();
      await userEvent.click(screen.getByRole("button",{name:"Add model provider"}));
      expect(await screen.findByRole("alert")).toHaveTextContent("Your input has been kept");
      expect(screen.getByLabelText("Supplier API Key")).toHaveValue("test-secret");
      expect(screen.getByLabelText("Provider name")).toHaveValue("test-provider");
      expect(screen.getByRole("button",{name:"Add model provider"})).toBeEnabled();
    });
  }
  /** 验证异步保存禁重复提交和取消，完成后恢复；仅使用可控 Promise，自动清理。 */
  it("waits for asynchronous submission", async () => {
    let resolve!:()=>void; const submit=vi.fn(()=>new Promise<void>(r=>{resolve=r;}));
    renderModal({onSubmit:submit}); await choose("New Supplier"); await fillSupplier();
    await userEvent.click(screen.getByRole("button",{name:"Add model provider"}));
    expect(screen.getByRole("button",{name:"Cancel"})).toBeDisabled();
    expect(submit).toHaveBeenCalledTimes(1); resolve();
    await waitFor(()=>expect(screen.getByRole("button",{name:"Cancel"})).toBeEnabled());
  });
  /** 验证目录失败提示与重试恢复，前置首次拒绝请求；独立查询缓存自动清理。 */
  it("retries a failed metadata request", async () => {
    vi.mocked(getProviderCreateMetadata).mockRejectedValueOnce(new Error("offline")); renderModal();
    expect(await screen.findByRole("alert")).toHaveTextContent("Failed to load");
    await userEvent.click(screen.getByRole("button",{name:"Retry"}));
    await choose("New Supplier"); expect(screen.getByLabelText("Supplier API Key")).toBeVisible();
  });
});
