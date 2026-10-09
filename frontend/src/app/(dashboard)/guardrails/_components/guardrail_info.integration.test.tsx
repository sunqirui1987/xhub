import * as networking from "@/components/networking";
import { fireEvent, render, waitFor, screen } from "@testing-library/react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { setActiveLocale } from "@/i18n";
import GuardrailInfoView from "./guardrail_info";

// 仅替换网络边界；详情、外部表单、本地表单和 XGo 弹窗使用真实组件。
vi.mock("@/components/networking", () => ({
  getGuardrailInfo: vi.fn(),
  getGuardrailUISettings: vi.fn(),
  getGuardrailProviderSpecificParams: vi.fn(),
  updateGuardrailCall: vi.fn(),
  applyGuardrail: vi.fn(),
  createGuardrailCall: vi.fn(),
  testCustomCodeGuardrail: vi.fn(),
}));
// 历史内容过滤器保留原测试替身，验证未修改字段不会被意外覆盖。
vi.mock("./content_filter/ContentFilterManager", () => ({
  default: ({ onUnsavedChanges, onDataChange, isEditing }: any) => (
    <div>
      {isEditing && (
        <button
          onClick={() => {
            onUnsavedChanges(true);
            onDataChange?.(["new_pattern"], ["new_word"], []);
          }}
        >
          Simulate Change
        </button>
      )}
    </div>
  ),
  formatContentFilterDataForAPI: (patterns: any[], blockedWords: any[], categories?: any[]) => ({
    patterns,
    blocked_words: blockedWords,
    categories: categories ?? [],
  }),
}));
beforeEach(() => {
  vi.clearAllMocks();
  setActiveLocale("en");
  vi.mocked(networking.getGuardrailUISettings).mockResolvedValue({
    supported_entities: [],
    supported_actions: [],
    pii_entity_categories: [],
    supported_modes: ["pre_call"],
  });
  vi.mocked(networking.getGuardrailProviderSpecificParams).mockResolvedValue({});
});
/**
 * 用途：构造服务器详情响应；每次调用返回独立对象，避免测试间状态串扰。
 * 参数：provider：执行器标识；location：数据库或配置文件。
 * 返回：详情响应对象，含可识别的名称和时间。
 * 调用：本文件详情连线测试。
 * 测试：由下列各场景使用；不伪造外部服务已支持的阶段。
 */
function detail(provider: string, location = "database") {
  return {
    guardrail_id: "123",
    guardrail_name: "Test Guardrail",
    litellm_params: { guardrail: provider, mode: "pre_call", default_on: true },
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    guardrail_definition_location: location,
  };
}
describe("Guardrail Info", () => {
  it("外部详情显示真实服务字段并测试当前配置", async () => {
    vi.mocked(networking.getGuardrailInfo).mockResolvedValue(detail("presidio"));
    vi.mocked(networking.applyGuardrail).mockResolvedValue({ action: "block", blocked: true } as never);
    render(<GuardrailInfoView guardrailId="123" onClose={() => {}} accessToken="123" isAdmin />);
    expect(await screen.findByRole("heading", { name: "Presidio PII" })).toBeInTheDocument();
    expect(screen.getByLabelText("Guardrail name")).toHaveValue("Test Guardrail");
    fireEvent.change(screen.getByLabelText("Analyzer API Base *"), { target: { value: "https://analyzer.example" } });
    fireEvent.change(screen.getByLabelText("Test text"), { target: { value: "sensitive" } });
    fireEvent.click(screen.getByRole("button", { name: "Test guardrail" }));
    await waitFor(() =>
      expect(networking.applyGuardrail).toHaveBeenCalledWith(
        "123",
        "Test Guardrail",
        "sensitive",
        null,
        null,
        null,
        expect.objectContaining({
          litellm_params: expect.objectContaining({
            guardrail: "presidio",
            mode: "pre_call",
            api_base: "https://analyzer.example",
          }),
        }),
      ),
    );
    expect(await screen.findByText(/"blocked": true/)).toBeInTheDocument();
  });
  it("配置文件外部护栏只读且保留已保存配置调试", async () => {
    vi.mocked(networking.getGuardrailInfo).mockResolvedValue(detail("presidio", "config"));
    render(<GuardrailInfoView guardrailId="123" onClose={() => {}} accessToken="123" isAdmin />);
    expect(await screen.findByLabelText("Guardrail name")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save guardrail" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test guardrail" })).toBeEnabled();
  });
  it("本地详情进入与 XGo 一致的配置和参考布局", async () => {
    vi.mocked(networking.getGuardrailInfo).mockResolvedValue({
      ...detail("local"),
      litellm_params: { ...detail("local").litellm_params, blocked_words: ["secret"] },
    });
    render(<GuardrailInfoView guardrailId="123" onClose={() => {}} accessToken="123" isAdmin />);
    expect(await screen.findByRole("complementary", { name: "Rule reference" })).toBeInTheDocument();
    expect(screen.getByLabelText("Keywords (one per line)")).toHaveValue("secret");
  });
  it("should handle content filter updates correctly", async () => {
    // Mock the network responses
    vi.mocked(networking.getGuardrailInfo).mockResolvedValue({
      guardrail_id: "123",
      guardrail_name: "Content Filter Guardrail",
      litellm_params: {
        guardrail: "litellm_content_filter",
        mode: "pre_call",
        default_on: true,
        patterns: ["initial_pattern"],
        blocked_words: ["initial_word"],
      },
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      guardrail_definition_location: "database",
    });

    vi.mocked(networking.getGuardrailUISettings).mockResolvedValue({
      supported_entities: [],
      supported_actions: [],
      pii_entity_categories: [],
      supported_modes: ["pre_call", "post_call"],
    });

    vi.mocked(networking.getGuardrailProviderSpecificParams).mockResolvedValue({});
    vi.mocked(networking.updateGuardrailCall).mockResolvedValue({ status: "success" });

    render(<GuardrailInfoView guardrailId="123" onClose={() => {}} accessToken="123" isAdmin={true} />);

    await waitFor(() => {
      expect(screen.getByText("Settings")).toBeInTheDocument();
    });

    // Go to Settings tab
    fireEvent.click(screen.getByText("Settings"));

    await waitFor(() => {
      expect(screen.getByText("Guardrail Settings")).toBeInTheDocument();
    });

    // Enter Edit Mode
    fireEvent.click(screen.getByText("Edit Settings"));

    // Modify Guardrail Name to force an update
    const nameInput = screen.getByLabelText("Guardrail Name");
    fireEvent.change(nameInput, { target: { value: "Updated Name" } });

    // Save with only name change
    const saveButton = screen.getByText("Save Changes");
    fireEvent.click(saveButton);

    await waitFor(() => {
      expect(networking.updateGuardrailCall).toHaveBeenCalled();
    });

    // Verify call did NOT include patterns or blocked_words (because no changes)
    // updateGuardrailCall(accessToken, guardrailId, updateData) -> index 2 is updateData
    const firstCallArgs: any = vi.mocked(networking.updateGuardrailCall).mock.calls[0][2];

    // Verify attributes that definitely changed
    expect(firstCallArgs.guardrail_name).toBe("Updated Name");

    // litellm_params might be undefined if empty, which is correct.
    // If it exists, ensure patterns/blocked_words are not in it.
    if (firstCallArgs.litellm_params) {
      expect(firstCallArgs.litellm_params.patterns).toBeUndefined();
      expect(firstCallArgs.litellm_params.blocked_words).toBeUndefined();
    }

    // Clear mocks to reset call count
    vi.clearAllMocks();

    // Enter Edit Mode again to make changes
    await waitFor(() => {
      expect(screen.getByText("Edit Settings")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByText("Edit Settings"));

    // Now modify the values using the mock button
    const simulateChangeButton = screen.getByText("Simulate Change");
    fireEvent.click(simulateChangeButton);

    // Save again
    fireEvent.click(screen.getByText("Save Changes"));

    await waitFor(() => {
      expect(networking.updateGuardrailCall).toHaveBeenCalled();
    });

    // Verify call INCLUDES patterns and blocked_words
    const secondCallArgs: any = vi.mocked(networking.updateGuardrailCall).mock.calls[0][2];
    expect(secondCallArgs.litellm_params).toBeDefined();
    expect(secondCallArgs.litellm_params.patterns).toEqual(["new_pattern"]);
    expect(secondCallArgs.litellm_params.blocked_words).toEqual(["new_word"]);
  });
});
describe("Guardrail Info when the guardrail cannot be loaded", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("should keep Back to Guardrails reachable so a stale ?guardrail= link is not a dead end", async () => {
    vi.mocked(networking.getGuardrailInfo).mockRejectedValue(new Error("Guardrail stale-id not found"));
    vi.mocked(networking.getGuardrailUISettings).mockResolvedValue({
      supported_entities: [],
      supported_actions: [],
      pii_entity_categories: [],
      supported_modes: [],
    });
    vi.mocked(networking.getGuardrailProviderSpecificParams).mockResolvedValue({});
    const onClose = vi.fn();

    render(<GuardrailInfoView guardrailId="stale-id" onClose={onClose} accessToken="123" isAdmin={true} />);

    expect(await screen.findByText("Guardrail not found")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /back to guardrails/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
