import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import CompareUI from "./CompareUI";
import { makeOpenAIChatCompletionRequest } from "@/components/llm_calls/chat_completion";

vi.mock("@/components/llm_calls/fetch_models", () => ({
  fetchAvailableModels: vi.fn().mockResolvedValue([
    {
      model_group: "gpt-4",
      endpoints: [
        {
          endpoint_id: "chat",
          transport: "bypass_openai_chat",
          kind: "adapted",
          protocol: "adapted",
          family: "chat",
          method: "POST",
          path: "/v1/chat/completions",
        },
      ],
    },
    {
      model_group: "gpt-3.5-turbo",
      endpoints: [
        {
          endpoint_id: "chat",
          transport: "bypass_openai_chat",
          kind: "adapted",
          protocol: "adapted",
          family: "chat",
          method: "POST",
          path: "/v1/chat/completions",
        },
      ],
    },
  ]),
}));

vi.mock("@/components/llm_calls/chat_completion", () => ({
  makeOpenAIChatCompletionRequest: vi.fn().mockResolvedValue(undefined),
}));

let capturedOnImageUpload: ((file: File) => false) | null = null;

vi.mock("../chat_ui/ChatImageUpload", () => ({
  default: ({ onImageUpload }: { onImageUpload: (file: File) => false }) => {
    capturedOnImageUpload = onImageUpload;
    return (
      <div data-testid="chat-image-upload">
        <button data-testid="trigger-upload">Upload</button>
      </div>
    );
  },
}));

vi.mock("../chat_ui/ChatImageUtils", () => ({
  createChatMultimodalMessage: vi.fn().mockResolvedValue({
    role: "user",
    content: [
      { type: "text", text: "test message" },
      { type: "image_url", image_url: { url: "data:image/png;base64,test" } },
    ],
  }),
  createChatDisplayMessage: vi.fn().mockReturnValue({
    role: "user",
    content: "test message [Image attached]",
    imagePreviewUrl: "blob:test-url",
  }),
}));

vi.mock("./components/ComparisonPanel", () => ({
  ComparisonPanel: ({ comparison, onRemove }: { comparison: any; onRemove: () => void }) => (
    <div data-testid={`comparison-panel-${comparison.id}`}>
      {comparison.failure && <span role="alert">{comparison.failure.title}</span>}
      <span>{comparison.messages.map((m: any) => m.content).join("|")}</span>

      <span data-testid={`endpoint-${comparison.id}`}>{comparison.endpoint}</span>
      <button data-testid={`remove-${comparison.id}`} onClick={onRemove}>
        Remove
      </button>
    </div>
  ),
}));

vi.mock("./components/MessageInput", () => ({
  MessageInput: ({ value, onChange, onSend, disabled, hasAttachment, uploadComponent }: any) => (
    <div data-testid="message-input">
      {uploadComponent && <div data-testid="upload-component">{uploadComponent}</div>}
      <textarea
        data-testid="message-textarea"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
      />
      <button data-testid="send-button" onClick={onSend} disabled={disabled}>
        Send
      </button>
      {hasAttachment && <div data-testid="has-attachment">Attachment</div>}
    </div>
  ),
}));

beforeEach(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
  global.URL.createObjectURL = vi.fn().mockReturnValue("blob:test-url");
  global.URL.revokeObjectURL = vi.fn();
  capturedOnImageUpload = null;
  vi.clearAllMocks();
  vi.mocked(makeOpenAIChatCompletionRequest).mockResolvedValue(undefined);
});

describe("CompareUI", () => {
  it("should render", () => {
    render(<CompareUI accessToken="test-token" disabledPersonalKeyCreation={false} />);
    expect(screen.getByTestId("comparison-panel-1")).toBeInTheDocument();
    expect(screen.getByTestId("comparison-panel-2")).toBeInTheDocument();
    expect(screen.getByTestId("message-input")).toBeInTheDocument();
  });

  it("adds a comparison when Add Comparison button is clicked", async () => {
    const user = userEvent.setup();
    const { container } = render(<CompareUI accessToken="test-token" disabledPersonalKeyCreation={false} />);

    // Verify initial state: 2 comparison panels
    expect(screen.getByTestId("comparison-panel-1")).toBeInTheDocument();
    expect(screen.getByTestId("comparison-panel-2")).toBeInTheDocument();
    let comparisonPanels = container.querySelectorAll('[data-testid^="comparison-panel-"]');
    expect(comparisonPanels).toHaveLength(2);

    await user.click(screen.getByRole("button", { name: /Add Comparison/i }));

    // Wait for the new comparison panel to be added (should have 3 total now)
    await waitFor(() => {
      comparisonPanels = container.querySelectorAll('[data-testid^="comparison-panel-"]');
      expect(comparisonPanels).toHaveLength(3);
    });

    // Verify the original 2 panels are still there
    expect(screen.getByTestId("comparison-panel-1")).toBeInTheDocument();
    expect(screen.getByTestId("comparison-panel-2")).toBeInTheDocument();
  });

  it("should handle image upload and send message with attachment", async () => {
    const user = userEvent.setup();
    render(<CompareUI accessToken="test-token" disabledPersonalKeyCreation={false} />);

    const file = new File(["test content"], "test-image.png", { type: "image/png" });

    await waitFor(() => {
      expect(capturedOnImageUpload).not.toBeNull();
    });

    if (capturedOnImageUpload) {
      capturedOnImageUpload(file);
    }

    await waitFor(() => {
      expect(screen.getByTestId("has-attachment")).toBeInTheDocument();
    });

    const textarea = screen.getByTestId("message-textarea");
    fireEvent.change(textarea, { target: { value: "Describe this image" } });

    const sendButton = screen.getByTestId("send-button");
    expect(sendButton).toBeEnabled();
    await user.click(sendButton);

    await waitFor(() => {
      expect(makeOpenAIChatCompletionRequest).toHaveBeenCalled();
    });
  });
});

/** 前置独立请求回调；验证失败卡回滚、成功卡保留结果及公共草稿；自动卸载，无后台写入。 */
it("失败卡片独立保留诊断和公共草稿", async () => {
  vi.mocked(makeOpenAIChatCompletionRequest).mockImplementation(async (_history, update, model) => {
    if (model === "gpt-4") throw new Error("This model has no upstream API key configured.");
    update("local-ok", model);
  });
  render(<CompareUI accessToken="test-token" disabledPersonalKeyCreation={false} />);
  await waitFor(() => expect(screen.getByRole("button", { name: "Add Comparison" })).toBeEnabled());
  fireEvent.change(screen.getByTestId("message-textarea"), { target: { value: "keep-draft" } });
  fireEvent.click(screen.getByTestId("send-button"));
  expect(await screen.findByRole("alert")).toHaveTextContent("Upstream key is missing");
  expect(screen.getByText(/local-ok/)).toBeInTheDocument();
  expect(screen.getByTestId("message-textarea")).toHaveValue("keep-draft");
  await waitFor(() => expect(screen.getByTestId("send-button")).toBeEnabled());
  const count = vi.mocked(makeOpenAIChatCompletionRequest).mock.calls.length;
  vi.mocked(makeOpenAIChatCompletionRequest).mockImplementation(async (_history, update, model) => {
    update("recovered", model);
  });
  fireEvent.click(screen.getByTestId("send-button"));
  await waitFor(() => expect(screen.getByTestId("message-textarea")).toHaveValue(""));
  expect(vi.mocked(makeOpenAIChatCompletionRequest).mock.calls.length).toBe(count + 1);
  expect(vi.mocked(makeOpenAIChatCompletionRequest).mock.calls.at(-1)?.[2]).toBe("gpt-4");
});
/** 前置挂起请求并捕获回调；验证停止中止信号、阻止迟到内容及恢复入口；自动卸载，无外部数据。 */
it("停止全部阻止迟到回调", async () => {
  const callbacks: Array<() => void> = [];
  const signals: AbortSignal[] = [];
  vi.mocked(makeOpenAIChatCompletionRequest).mockImplementation(
    (_history, update, _model, _key, _tags, signal) =>
      new Promise((resolve) => {
        signals.push(signal!);
        callbacks.push(() => {
          update("late-result");
          resolve();
        });
      }),
  );
  render(<CompareUI accessToken="test-token" disabledPersonalKeyCreation={false} />);
  await waitFor(() => expect(screen.getByTestId("endpoint-1")).toHaveTextContent("/v1/chat/completions"));
  fireEvent.change(screen.getByTestId("message-textarea"), { target: { value: "stop-draft" } });
  fireEvent.click(screen.getByTestId("send-button"));
  await waitFor(() => expect(signals).toHaveLength(2));
  fireEvent.click(screen.getByRole("button", { name: "停止全部" }));
  expect(signals.every((signal) => signal.aborted)).toBe(true);
  await act(async () => callbacks.forEach((callback) => callback()));
  expect(screen.queryByText(/late-result/)).not.toBeInTheDocument();
  expect(screen.getByTestId("send-button")).toBeEnabled();
  expect(screen.getByTestId("comparison-panel-1")).not.toHaveTextContent("stop-draft");
});
