import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { setActiveLocale } from "@/i18n";
import { applyGuardrail, createGuardrailCall } from "@/components/networking";
import GuardrailGarden from "./guardrail_garden";
import { PARTNER_GUARDRAIL_CARDS } from "./guardrail_garden_data";

// 仅替换网络边界；真实渲染花园与外部配置表单，验证目录到调试/保存的完整连线。
vi.mock("@/components/networking", () => ({
  applyGuardrail: vi.fn(),
  createGuardrailCall: vi.fn(),
  updateGuardrailCall: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  setActiveLocale("zh-CN");
});
const props = { accessToken: "token", onGuardrailCreated: vi.fn(), onCreateLocal: vi.fn(), onCreateXGo: vi.fn() };
describe("护栏花园外部接入", () => {
  it("保留所有伙伴目录并明确标记支持状态", () => {
    render(<GuardrailGarden {...props} />);
    for (const card of PARTNER_GUARDRAIL_CARDS) expect(screen.getByText(card.name)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Azure Content Safety/ })).toHaveTextContent("已接入");
    expect(screen.getByRole("button", { name: /Aporia/ })).toHaveTextContent("尚未移植");
    fireEvent.click(screen.getByRole("button", { name: /XGo 自定义护栏/ }));
    expect(props.onCreateXGo).toHaveBeenCalledOnce();
  });
  it("未移植服务可以进入远端 LiteLLM 配置", () => {
    render(<GuardrailGarden {...props} />);
    fireEvent.change(screen.getByLabelText("搜索护栏服务"), { target: { value: "Aporia" } });
    expect(screen.queryByText("Azure Content Safety")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Aporia/ }));
    expect(screen.getByText("尚未移植：可以通过远端 LiteLLM 使用该服务。")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "配置远端 LiteLLM" }));
    expect(screen.getByLabelText("远端护栏名称 *")).toBeInTheDocument();
    expect(screen.getByText("请先在远端 LiteLLM 配置 Aporia AI，再填写该护栏的远端名称。")).toBeInTheDocument();
  });
  it("Azure 未保存配置能调试，并以相同配置保存", async () => {
    vi.mocked(applyGuardrail).mockResolvedValue({ blocked: true, action: "block" } as never);
    vi.mocked(createGuardrailCall).mockResolvedValue({} as never);
    render(<GuardrailGarden {...props} />);
    fireEvent.click(screen.getByRole("button", { name: /Azure Content Safety/ }));
    fireEvent.change(screen.getByLabelText("护栏名称"), { target: { value: "azure-policy" } });
    fireEvent.change(screen.getByLabelText("Azure endpoint *"), { target: { value: "https://test.azure.com" } });
    fireEvent.change(screen.getByLabelText("API 密钥 *"), { target: { value: "test-key" } });
    fireEvent.change(screen.getByLabelText("测试文本"), { target: { value: "unsafe" } });
    fireEvent.click(screen.getByRole("button", { name: "测试护栏" }));
    const payload = expect.objectContaining({
      guardrail_name: "azure-policy",
      litellm_params: expect.objectContaining({
        guardrail: "azure/text_moderations",
        api_key: "test-key",
        severity_threshold: 4,
        mode: "pre_call",
      }),
    });
    await waitFor(() =>
      expect(applyGuardrail).toHaveBeenCalledWith("token", "azure-policy", "unsafe", null, null, null, payload),
    );
    expect(await screen.findByText(/"blocked": true/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "保存护栏" }));
    await waitFor(() => expect(createGuardrailCall).toHaveBeenCalledWith("token", payload));
  });
});
