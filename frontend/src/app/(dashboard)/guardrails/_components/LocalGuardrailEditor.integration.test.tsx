import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { setActiveLocale } from "@/i18n";
import { applyGuardrail, createGuardrailCall } from "@/components/networking";
import { GuardrailDefinitionLocation } from "@/components/guardrails/types";
import LocalGuardrailModal from "./LocalGuardrailEditor";

// 网络是唯一替身；真实表单证明草稿调试、保存和只读限制相互一致。
vi.mock("@/components/networking", () => ({
  applyGuardrail: vi.fn(),
  createGuardrailCall: vi.fn(),
  updateGuardrailCall: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  setActiveLocale("zh-CN");
});
const props = { visible: true, accessToken: "token", onSuccess: vi.fn(), onClose: vi.fn() };
describe("关键词 / 正则编辑器", () => {
  it("保存前测试模板，并用同一配置保存", async () => {
    vi.mocked(applyGuardrail).mockResolvedValue({ action: "modify", response_text: "[手机号已隐藏]" } as never);
    vi.mocked(createGuardrailCall).mockResolvedValue({} as never);
    render(<LocalGuardrailModal {...props} />);
    expect(screen.getByRole("complementary", { name: "规则参考" })).toBeInTheDocument();
    // 回归截图中的问题：执行模式必须能展开，未接通的阶段必须明确不可选。
    const mode = screen.getByRole("combobox", { name: "执行模式" });
    expect(mode).toBeEnabled();
    fireEvent.click(mode);
    expect(await screen.findByRole("listbox")).toBeVisible();
    expect(screen.getByRole("option", { name: "调用后（响应） · post_call（暂不支持）" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    fireEvent.click(screen.getByRole("option", { name: "调用前（请求） · pre_call" }));
    fireEvent.change(screen.getByLabelText("护栏名称"), { target: { value: "phone-policy" } });
    fireEvent.change(screen.getByLabelText("模板"), { target: { value: "phone" } });
    fireEvent.change(screen.getByLabelText("测试文本"), { target: { value: "13800138000" } });
    fireEvent.click(screen.getByRole("button", { name: "测试当前规则" }));
    await waitFor(() =>
      expect(applyGuardrail).toHaveBeenCalledWith(
        "token",
        "phone-policy",
        "13800138000",
        null,
        null,
        null,
        expect.objectContaining({
          litellm_params: expect.objectContaining({
            action: "redact",
            patterns: ["1[3-9][0-9]{9}"],
            replacement: "[手机号已隐藏]",
            mode: "pre_call",
            skip_system_message_in_guardrail: null,
          }),
        }),
      ),
    );
    expect(await screen.findByLabelText("测试结果")).toHaveTextContent("[手机号已隐藏]");
    fireEvent.click(screen.getByRole("button", { name: "保存护栏" }));
    await waitFor(() =>
      expect(createGuardrailCall).toHaveBeenCalledWith("token", vi.mocked(applyGuardrail).mock.calls[0][6]),
    );
  });
  it("只读规则不能保存，调试使用服务器配置", async () => {
    vi.mocked(applyGuardrail).mockResolvedValue({ action: "allow" } as never);
    render(
      <LocalGuardrailModal
        {...props}
        readOnly
        rule={{
          guardrail_id: "saved",
          guardrail_name: "policy",
          guardrail_info: {},
          guardrail_definition_location: GuardrailDefinitionLocation.DB,
          litellm_params: { guardrail: "local", mode: "pre_call", default_on: false, blocked_words: ["secret"] },
        }}
      />,
    );
    expect(screen.getByLabelText("护栏名称")).toBeDisabled();
    expect(screen.getByRole("button", { name: "保存护栏" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("测试文本"), { target: { value: "safe" } });
    fireEvent.click(screen.getByRole("button", { name: "测试当前规则" }));
    await waitFor(() =>
      expect(applyGuardrail).toHaveBeenCalledWith("token", "policy", "safe", null, null, null, undefined),
    );
  });
});
