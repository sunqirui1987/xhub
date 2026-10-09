import { setActiveLocale } from "@/i18n";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import CustomCodeModal from "./CustomCodeModal";
import { copyToClipboard } from "@/utils/dataUtils";
import { createGuardrailCall, testCustomCodeGuardrail } from "@/components/networking";
vi.mock("@/components/networking", () => ({
  createGuardrailCall: vi.fn(),
  updateGuardrailCall: vi.fn(),
  testCustomCodeGuardrail: vi.fn(),
}));
vi.mock("@/utils/dataUtils", () => ({ copyToClipboard: vi.fn() }));
beforeEach(() => vi.clearAllMocks());
const props = { visible: true, onClose: vi.fn(), onSuccess: vi.fn(), accessToken: "token" };
describe("XGo custom guardrail editor", () => {
  it("saves Go syntax and the supported execution stage", async () => {
    vi.mocked(createGuardrailCall).mockResolvedValue({} as never);
    render(<CustomCodeModal {...props} />);
    expect((screen.getByLabelText("XGo 代码") as HTMLTextAreaElement).value).toContain("func ApplyGuardrail(");
    expect(screen.queryByLabelText(/Python|PYTHON/)).not.toBeInTheDocument();
    expect((screen.getByLabelText("XGo 代码") as HTMLTextAreaElement).value).toContain("for text <- texts");
    expect(screen.getByRole("button", { name: "保存护栏" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("护栏名称"), { target: { value: "secret-policy" } });
    fireEvent.click(screen.getByRole("button", { name: "保存护栏" }));
    await waitFor(() =>
      expect(createGuardrailCall).toHaveBeenCalledWith(
        "token",
        expect.objectContaining({
          guardrail_name: "secret-policy",
          litellm_params: expect.objectContaining({
            guardrail: "custom_code",
            custom_code_language: "xgo",
            mode: "pre_call",
          }),
        }),
      ),
    );
  });
  it("tests the current unsaved template and shows block as block", async () => {
    vi.mocked(testCustomCodeGuardrail).mockResolvedValue({
      success: true,
      result: { action: "block", reason: "敏感文本" },
    });
    render(<CustomCodeModal {...props} />);
    fireEvent.change(screen.getByLabelText("模板"), { target: { value: "redact" } });
    expect((screen.getByLabelText("XGo 代码") as HTMLTextAreaElement).value).toContain("return Modify(texts)");
    fireEvent.click(screen.getByRole("button", { name: "测试当前代码" }));
    await waitFor(() =>
      expect(testCustomCodeGuardrail).toHaveBeenCalledWith(
        "token",
        expect.objectContaining({
          input_type: "request",
          test_input: { texts: ["hello secret"], model: "your-model" },
          request_data: { model: "your-model", metadata: {} },
        }),
      ),
    );
    expect(await screen.findByLabelText("测试结果")).toHaveTextContent('"action": "block"');
  });
  it("copies an executable primitive example", async () => {
    vi.mocked(copyToClipboard).mockResolvedValue(true);
    render(<CustomCodeModal {...props} />);
    fireEvent.click(screen.getByRole("button", { name: "复制 Block(reason) 示例" }));
    expect(copyToClipboard).toHaveBeenCalledWith('return Block("文本包含敏感内容")', expect.any(String));
    expect(await screen.findByRole("status")).toHaveTextContent("已复制 Block(reason) 示例");
  });
  it("passes edited model and metadata to the tester and rejects malformed input", async () => {
    vi.mocked(testCustomCodeGuardrail).mockResolvedValue({ success: true, result: { action: "allow" } });
    render(<CustomCodeModal {...props} />);
    const input = screen.getByLabelText("测试输入（JSON）");
    fireEvent.change(input, { target: { value: '{"texts":[1]}' } });
    fireEvent.click(screen.getByRole("button", { name: "测试当前代码" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("texts 必须是字符串数组");
    expect(testCustomCodeGuardrail).not.toHaveBeenCalled();
    fireEvent.change(input, {
      target: { value: JSON.stringify({ texts: ["hello"], model: "demo", metadata: { tier: "test" } }) },
    });
    fireEvent.click(screen.getByRole("button", { name: "测试当前代码" }));
    await waitFor(() =>
      expect(testCustomCodeGuardrail).toHaveBeenCalledWith(
        "token",
        expect.objectContaining({
          test_input: { texts: ["hello"], model: "demo" },
          request_data: { model: "demo", metadata: { tier: "test" } },
        }),
      ),
    );
    expect(await screen.findByLabelText("测试结果")).toHaveTextContent('"action": "allow"');
  });
  it("shows compiler errors and keeps YAML configuration read only", async () => {
    vi.mocked(testCustomCodeGuardrail).mockResolvedValue({
      success: false,
      error: "invalid Go syntax",
      error_type: "compilation",
    });
    render(
      <CustomCodeModal
        {...props}
        readOnly
        editData={{ guardrail_id: "yaml", guardrail_name: "yaml", litellm_params: {} }}
      />,
    );
    expect(screen.getByRole("button", { name: "保存护栏" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "测试当前代码" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("invalid Go syntax");
    expect(createGuardrailCall).not.toHaveBeenCalled();
  });
});

beforeEach(() => setActiveLocale("zh-CN"));
