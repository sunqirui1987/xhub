import { translate } from "@/i18n";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders as render } from "@/../tests/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ChatUI from "./ChatUI";
import { endpointLabel } from "@/components/llm_calls/model_endpoints";
import * as fetchModelsModule from "@/components/llm_calls/fetch_models";
import { makeOpenAIChatCompletionRequest } from "@/components/llm_calls/chat_completion";
import { makeAnthropicMessagesRequest } from "../../llm_calls/anthropic_messages";

vi.mock("@/components/llm_calls/fetch_models", () => ({
  fetchAvailableModels: vi.fn(),
}));

vi.mock("@/components/llm_calls/chat_completion", () => ({
  makeOpenAIChatCompletionRequest: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("../../llm_calls/anthropic_messages", () => ({
  makeAnthropicMessagesRequest: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@/components/networking", () => ({
  tagListCall: vi.fn().mockResolvedValue({}),
  vectorStoreListCall: vi.fn().mockResolvedValue({ data: [] }),
  getGuardrailsList: vi.fn().mockResolvedValue({ data: [] }),
  getPoliciesList: vi.fn().mockResolvedValue({ data: [] }),
  modelHubCall: vi.fn().mockResolvedValue({ data: [] }),
  fetchMCPServers: vi.fn().mockResolvedValue([]),
  fetchMCPToolsets: vi.fn().mockResolvedValue([]),
  listMCPTools: vi.fn().mockResolvedValue({ tools: [] }),
  callMCPTool: vi.fn(),
  getProxyBaseUrl: vi.fn().mockReturnValue("http://localhost:4000"),
}));

beforeEach(() => {
  Element.prototype.scrollIntoView = () => {};
});

const CHAT_REQUEST_ARG_COUNT = 26;
const STREAMING_ENABLED_ARG_INDEX = 25;
const MESSAGES_REQUEST_ARG_COUNT = 19;
const MESSAGES_STREAMING_ENABLED_ARG_INDEX = 18;

async function openComboboxByPlaceholder(placeholder: string) {
  const user = userEvent.setup();
  const combobox = await screen.findByPlaceholderText(placeholder);
  await user.click(combobox);
  return combobox;
}

async function selectComboboxOption(placeholder: string, optionLabel: string) {
  const user = userEvent.setup();
  await openComboboxByPlaceholder(placeholder);
  const option = await screen.findByRole("option", {
    name: optionLabel.startsWith("/") ? new RegExp(optionLabel + "$") : optionLabel,
  });
  await user.click(option);
}

/** endpoint 构造公开契约测试夹具；只登记指定路径，不用模型分类推断能力。 */
const endpoint = (path: string, kind: "adapted" | "bypass" = "adapted", protocol = "adapted") => ({
  endpoint_id: protocol,
  transport: kind,
  kind,
  protocol,
  family: "text",
  method: "POST",
  path,
});

describe("ChatUI", () => {
  /** 前置保存的模型、端点和完成历史；验证初始化恢复不清空历史，切换模型后清空；自动卸载并清理存储。 */
  it("恢复历史保留绑定，切换模型才重置", async () => {
    sessionStorage.setItem("selectedModel", "Model 1");
    sessionStorage.setItem("endpointPath", "/v1/chat/completions");
    sessionStorage.setItem("chatHistory", JSON.stringify([{ role: "assistant", content: "restored-answer" }]));
    sessionStorage.setItem("codeInterpreterEnabled", "{bad");
    render(
      <ChatUI accessToken="test" token="test" userRole="user" userID="test" disabledPersonalKeyCreation={false} />,
    );
    await waitFor(() =>
      expect(screen.getByPlaceholderText("Select an endpoint")).toHaveValue(
        endpointLabel(endpoint("/v1/chat/completions")),
      ),
    );
    expect(screen.getByText("restored-answer")).toBeInTheDocument();
    await selectComboboxOption("Select a Model", "Model 2");
    expect(screen.queryByText("restored-answer")).not.toBeInTheDocument();
  });
  /** 前置会话与本地模型夹具；验证模型优先、连接默认收起并可展开，无外部写入，环境卸载清理。 */
  it("连接设置默认收起且可按需展开", async () => {
    render(
      <ChatUI accessToken="test" token="test" userRole="user" userID="test" disabledPersonalKeyCreation={false} />,
    );
    expect(screen.getByRole("heading", { name: translate("en", "myModels.modelsAndEndpoints") })).toBeVisible();
    expect(screen.getByLabelText("Virtual Key Source")).not.toBeVisible();
    await userEvent.click(screen.getByText(translate("en", "myModels.connectionSettings")));
    expect(screen.getByLabelText("Virtual Key Source")).toBeVisible();
  });

  /** 前置三个显式部署夹具；验证列表、切换、无绑定状态与禁止发送，自动卸载清理。 */
  it("只列出所选模型的端点，切换模型后清除失效绑定", async () => {
    render(
      <ChatUI accessToken="test" token="test" userRole="user" userID="test" disabledPersonalKeyCreation={false} />,
    );
    expect(screen.getByPlaceholderText("Select an endpoint")).toBeDisabled();
    await selectComboboxOption(translate("en", "Select a Model"), "Model 1");
    await openComboboxByPlaceholder("Select an endpoint");
    expect(screen.getByRole("option", { name: /\/v1\/messages$/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /\/v1\/audio\/speech$/ })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("option", { name: /\/v1\/messages$/ }));
    await selectComboboxOption(translate("en", "Select a Model"), "Model 2");
    expect(screen.getByPlaceholderText("Select an endpoint")).toHaveValue(endpointLabel(endpoint("/v1/audio/speech")));
    await selectComboboxOption(translate("en", "Select a Model"), "Model 3");
    expect(screen.getByPlaceholderText("Select an endpoint")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Send message" })).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("该模型没有可调用的端点");
    expect(screen.queryByPlaceholderText("Describe the image you want to generate...")).not.toBeInTheDocument();
  });

  /** 前置原生与适配绑定及本地响应；验证精确路径和原生字段，不写外部数据，恢复 fetch。 */
  it("原生与适配 Responses 可以独立选择，并把原生请求发往实际路径", async () => {
    vi.mocked(fetchModelsModule.fetchAvailableModels).mockResolvedValueOnce([
      {
        model_group: "NativeModel",
        endpoints: [endpoint("/v1/responses"), endpoint("/bypass/openai/v1/responses", "bypass", "openai-responses")],
      },
    ]);
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ status: "completed", output: [{ content: [{ text: "原生结果" }] }] }), {
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    try {
      render(
        <ChatUI accessToken="test" token="test" userRole="user" userID="test" disabledPersonalKeyCreation={false} />,
      );
      await selectComboboxOption(translate("en", "Select a Model"), "NativeModel");
      await selectComboboxOption("Select an endpoint", "/bypass/openai/v1/responses");
      expect(screen.queryByText("Guardrails")).not.toBeInTheDocument();
      const input = screen.getByPlaceholderText("Type your message... (Shift+Enter for new line)");
      fireEvent.change(input, { target: { value: "hello" } });
      fireEvent.keyDown(input, { key: "Enter", code: "Enter" });
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
      expect(fetchMock.mock.calls[0][0]).toBe("http://localhost:4000/bypass/openai/v1/responses");
      const body = JSON.parse(fetchMock.mock.calls[0][1].body);
      expect(body.model).toBe("NativeModel");
      expect(body.input).toBeDefined();
      expect(body.guardrails).toBeUndefined();
      expect(await screen.findByText("原生结果")).toBeInTheDocument();
    } finally {
      vi.unstubAllGlobals();
    }
  });

  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();
    Element.prototype.scrollIntoView = vi.fn();

    (fetchModelsModule.fetchAvailableModels as ReturnType<typeof vi.fn>).mockResolvedValue([
      { model_group: "Model 1", mode: "chat", endpoints: [endpoint("/v1/chat/completions"), endpoint("/v1/messages")] },
      { model_group: "Model 2", endpoints: [endpoint("/v1/audio/speech"), endpoint("/v1/audio/transcriptions")] },
      { model_group: "Model 3", endpoints: [] },
    ]);
  });

  /** 前置网关缺少上游密钥；验证诊断独立于助手消息、草稿保留且重试上下文不含失败轮次；自动卸载清理。 */
  it("失败保留草稿并禁止错误污染上下文", async () => {
    vi.mocked(makeOpenAIChatCompletionRequest).mockRejectedValueOnce(
      Object.assign(new Error("This model has no upstream API key configured."), { status: 401 }),
    );
    render(
      <ChatUI accessToken="test" token="test" userRole="user" userID="test" disabledPersonalKeyCreation={false} />,
    );
    await selectComboboxOption("Select a Model", "Model 1");
    const input = screen.getByPlaceholderText("Type your message... (Shift+Enter for new line)");
    fireEvent.change(input, { target: { value: "retry draft" } });
    await userEvent.click(screen.getByRole("button", { name: "Send message" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Upstream key is missing");
    expect(input).toHaveValue("retry draft");
    expect(screen.queryByText("Assistant")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Send message" }));
    await waitFor(() => expect(makeOpenAIChatCompletionRequest).toHaveBeenCalledTimes(2));
    expect(vi.mocked(makeOpenAIChatCompletionRequest).mock.calls[1][0]).toEqual([
      { role: "user", content: "retry draft" },
    ]);
  });
  /** 前置未完成流式请求；验证清空触发取消且迟到文本不重新出现；回调本地完成，自动卸载清理。 */
  it("清空对话忽略迟到响应", async () => {
    let chunk!: (text: string, model?: string) => void;
    let finish!: () => void;
    vi.mocked(makeOpenAIChatCompletionRequest).mockImplementationOnce((_history, callback) => {
      chunk = callback;
      return new Promise<void>((resolve) => {
        finish = resolve;
      });
    });
    render(
      <ChatUI accessToken="test" token="test" userRole="user" userID="test" disabledPersonalKeyCreation={false} />,
    );
    await selectComboboxOption("Select a Model", "Model 1");
    fireEvent.change(screen.getByTestId("chat-composer-input"), { target: { value: "pending" } });
    await userEvent.click(screen.getByRole("button", { name: "Send message" }));
    const signal = vi.mocked(makeOpenAIChatCompletionRequest).mock.calls[0][5];
    await userEvent.click(screen.getByRole("button", { name: "Clear Chat" }));
    expect(signal?.aborted).toBe(true);
    await act(async () => {
      chunk("late result");
      finish();
    });
    expect(screen.queryByText("late result")).not.toBeInTheDocument();
  });
  it("should render the chat UI", async () => {
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );
    expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
  });

  it("should show the voice selector when the endpoint type is audio_speech", async () => {
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "Model 2");

    await waitFor(() => {
      expect(screen.getByText("Voice")).toBeInTheDocument();
      expect(screen.getByLabelText("Voice")).toBeInTheDocument();
    });
  });

  it("should show the SDK type by its human label rather than its wire value", async () => {
    const user = userEvent.setup();
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /get code/i }));

    const sdkTrigger = await screen.findByLabelText("SDK Type");
    expect(sdkTrigger).toHaveTextContent("OpenAI SDK");

    await user.click(sdkTrigger);
    await user.click(await screen.findByRole("option", { name: "Azure SDK" }));

    expect(await screen.findByLabelText("SDK Type")).toHaveTextContent("Azure SDK");
  });

  it("should show the voice by its human label rather than its wire value", async () => {
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "Model 2");

    await waitFor(() => {
      expect(screen.getByLabelText("Voice")).toHaveTextContent("Alloy - Professional and confident");
    });
  });

  /** 前置模型目录夹具；验证精简列表用模型名称即可选择，测试环境卸载清理，无外部数据。 */
  it("should allow the user to select a model", async () => {
    const user = userEvent.setup();
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await openComboboxByPlaceholder(translate("en", "Select a Model"));

    await waitFor(() => {
      expect(screen.getAllByText("Model 1").length).toBeGreaterThan(0);
    });
    await user.click(screen.getByRole("option", { name: "Model 1" }));
    expect(screen.getByPlaceholderText(translate("en", "Select a Model"))).toHaveValue("Model 1");

    await user.click(screen.getAllByRole("button", { name: "Clear" })[0]);
    const input = screen.getByPlaceholderText(translate("en", "myModels.selectValidEndpoint"));
    fireEvent.change(input, { target: { value: "Contract endpoint check" } });
    expect(screen.getByRole("button", { name: "Send message" })).toBeDisabled();
    fireEvent.keyDown(input, { key: "Enter", code: "Enter" });
    expect(input).toHaveValue("Contract endpoint check");
    expect(makeOpenAIChatCompletionRequest).not.toHaveBeenCalled();
    expect(sessionStorage.getItem("endpointType")).toBeNull();

    await selectComboboxOption(translate("en", "Select a Model"), "Model 1");
    expect(screen.getByRole("button", { name: "Send message" })).toBeEnabled();
  });

  it("should show Simulate failure to test fallbacks in Model Settings when chat endpoint is selected", async () => {
    const user = userEvent.setup();
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "Model 1");

    await waitFor(() => {
      expect(screen.getByTestId("model-settings-button")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("model-settings-button"));

    await waitFor(() => {
      expect(screen.getByText("Model Settings")).toBeInTheDocument();
      expect(screen.getByText(/Simulate failure to test fallbacks/i)).toBeInTheDocument();
    });

    const fallbacksCheckbox = screen.getByRole("checkbox", {
      name: /Simulate failure to test fallbacks/i,
    });
    expect(fallbacksCheckbox).not.toBeChecked();

    await user.click(fallbacksCheckbox);

    await waitFor(() => {
      expect(screen.getByRole("checkbox", { name: /Simulate failure to test fallbacks/i })).toBeChecked();
    });
  });

  it("should send the chat request non-streaming after Stream responses is unchecked", async () => {
    const user = userEvent.setup();
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "Model 1");

    await waitFor(() => {
      expect(screen.getByTestId("model-settings-button")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("model-settings-button"));

    const streamingCheckbox = await screen.findByRole("checkbox", { name: /Stream responses/i });
    expect(streamingCheckbox).toBeChecked();

    await user.click(streamingCheckbox);

    await waitFor(() => {
      expect(screen.getByRole("checkbox", { name: /Stream responses/i })).not.toBeChecked();
    });

    const messageInput = await screen.findByPlaceholderText("Type your message... (Shift+Enter for new line)");
    await act(async () => {
      fireEvent.change(messageInput, { target: { value: "hello" } });
    });
    await act(async () => {
      fireEvent.keyDown(messageInput, { key: "Enter", code: "Enter" });
    });

    await waitFor(() => {
      expect(makeOpenAIChatCompletionRequest).toHaveBeenCalledTimes(1);
    });

    const requestArgs = vi.mocked(makeOpenAIChatCompletionRequest).mock.calls[0];
    expect(requestArgs).toHaveLength(CHAT_REQUEST_ARG_COUNT);
    expect(requestArgs[STREAMING_ENABLED_ARG_INDEX]).toBe(false);
  });

  it("should send the /v1/messages request non-streaming after Stream responses is unchecked", async () => {
    const user = userEvent.setup();
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "Model 1");
    await selectComboboxOption("Select an endpoint", "/v1/messages");

    await user.click(await screen.findByTestId("model-settings-button"));

    const streamingCheckbox = await screen.findByRole("checkbox", { name: /Stream responses/i });
    expect(streamingCheckbox).toBeChecked();
    await user.click(streamingCheckbox);

    await waitFor(() => {
      expect(screen.getByRole("checkbox", { name: /Stream responses/i })).not.toBeChecked();
    });

    const messageInput = await screen.findByPlaceholderText("Type your message... (Shift+Enter for new line)");
    await act(async () => {
      fireEvent.change(messageInput, { target: { value: "hello" } });
    });
    await act(async () => {
      fireEvent.keyDown(messageInput, { key: "Enter", code: "Enter" });
    });

    await waitFor(() => {
      expect(makeAnthropicMessagesRequest).toHaveBeenCalledTimes(1);
    });

    const requestArgs = vi.mocked(makeAnthropicMessagesRequest).mock.calls[0];
    expect(requestArgs).toHaveLength(MESSAGES_REQUEST_ARG_COUNT);
    expect(requestArgs[MESSAGES_STREAMING_ENABLED_ARG_INDEX]).toBe(false);
  });

  it("should force streaming in simplified mode even when the playground setting is off", async () => {
    sessionStorage.setItem("streamingEnabled", "false");

    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
        simplified
        fixedModel="Model 1"
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Chat")).toBeInTheDocument();
    });

    const messageInput = await screen.findByPlaceholderText("Type your message... (Shift+Enter for new line)");
    await act(async () => {
      fireEvent.change(messageInput, { target: { value: "hello" } });
    });
    await act(async () => {
      fireEvent.keyDown(messageInput, { key: "Enter", code: "Enter" });
    });

    await waitFor(() => {
      expect(makeOpenAIChatCompletionRequest).toHaveBeenCalledTimes(1);
    });

    const requestArgs = vi.mocked(makeOpenAIChatCompletionRequest).mock.calls[0];
    expect(requestArgs).toHaveLength(CHAT_REQUEST_ARG_COUNT);
    expect(requestArgs[STREAMING_ENABLED_ARG_INDEX]).toBe(true);
    expect(sessionStorage.getItem("streamingEnabled")).toBe("false");
  });

  it("should offer protocol settings for a responses-only model", async () => {
    const user = userEvent.setup();
    (fetchModelsModule.fetchAvailableModels as ReturnType<typeof vi.fn>).mockResolvedValue([
      { model_group: "ResponsesModel", endpoints: [endpoint("/v1/responses")] },
    ]);

    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "ResponsesModel");

    await waitFor(() => {
      expect(screen.getByTestId("model-settings-button")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("model-settings-button"));

    expect(await screen.findByRole("checkbox", { name: /Stream responses/i })).toBeChecked();
    expect(screen.getByText("Temperature")).toBeInTheDocument();
    expect(screen.getByText("Use Advanced Parameters")).toBeInTheDocument();
  });

  it("should show Fill button and populate customProxyBaseUrl when proxySettings.LITELLM_UI_API_DOC_BASE_URL is provided", async () => {
    const testProxyUrl = "http://localhost:5000";

    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
        proxySettings={{
          LITELLM_UI_API_DOC_BASE_URL: testProxyUrl,
        }}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    const fillButton = screen.getByText("Fill");
    expect(fillButton).toBeInTheDocument();

    act(() => {
      fireEvent.click(fillButton);
    });

    await waitFor(() => {
      expect(sessionStorage.getItem("customProxyBaseUrl")).toBe(testProxyUrl);
    });

    await waitFor(() => {
      expect(screen.queryByText("Fill")).not.toBeInTheDocument();
    });

    const customProxyInput = screen.getByPlaceholderText(
      "Optional: Enter custom proxy URL (e.g., http://localhost:5000)",
    );
    expect(customProxyInput).toHaveValue(testProxyUrl);
  });

  it("does not offer MCP servers in the playground sidebar", async () => {
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    expect(screen.queryByText("MCP Servers")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Select MCP servers")).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "/mcp-rest/tools/call" })).not.toBeInTheDocument();
  });

  it("should keep the chosen endpoint when a model that endpoint can serve is picked", async () => {
    (fetchModelsModule.fetchAvailableModels as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { model_group: "ChatModel", endpoints: [endpoint("/v1/responses")] },
    ]);

    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "ChatModel");

    expect(screen.getByPlaceholderText("Select an endpoint")).toHaveValue(endpointLabel(endpoint("/v1/responses")));
  });

  it("should list configured models before choosing an endpoint", async () => {
    (fetchModelsModule.fetchAvailableModels as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { model_group: "ChatModel", endpoints: [endpoint("/v1/responses")] },
      { model_group: "SpeechModel", mode: "audio_speech" },
    ]);

    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await openComboboxByPlaceholder(translate("en", "Select a Model"));

    await waitFor(() => {
      expect(screen.getAllByText("ChatModel").length).toBeGreaterThan(0);
    });
    expect(screen.getByText("SpeechModel")).toBeInTheDocument();
  });

  it("should attach an audio file dropped on the transcription upload area", async () => {
    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await selectComboboxOption(translate("en", "Select a Model"), "Model 2");
    await selectComboboxOption("Select an endpoint", "/v1/audio/transcriptions");

    const dropZone = (await screen.findByText("Click or drag audio file to upload")).closest("label");
    const file = new File(["clip"], "clip.wav", { type: "audio/wav" });
    fireEvent.drop(dropZone as HTMLElement, { dataTransfer: { files: [file] } });

    expect(await screen.findByText("clip.wav")).toBeInTheDocument();
  });

  /** 前置会话模型夹具；展开连接设置后验证密钥源中文/英文可读名称，无外部数据，卸载清理。 */
  it("should name the virtual key source options instead of showing raw values", async () => {
    const user = userEvent.setup();

    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await user.click(screen.getByText(translate("en", "myModels.connectionSettings")));
    const keySourceTrigger = screen.getByLabelText("Virtual Key Source");
    expect(keySourceTrigger).toHaveTextContent("Current UI Session");
    expect(keySourceTrigger).not.toHaveTextContent("session");

    await user.click(keySourceTrigger);
    await user.click(await screen.findByRole("option", { name: "Virtual Key" }));

    await waitFor(() => {
      expect(screen.getByLabelText("Virtual Key Source")).toHaveTextContent("Virtual Key");
    });
    expect(screen.getByLabelText("Virtual Key Source")).not.toHaveTextContent("custom");
  });

  /** 前置会话模型夹具；展开连接设置并切换自定义密钥，验证模型列表刷新，无外部数据，环境卸载清理。 */
  it("should re-enable the model selector when the virtual key is cleared mid-load", async () => {
    const user = userEvent.setup();
    (fetchModelsModule.fetchAvailableModels as ReturnType<typeof vi.fn>).mockImplementation(
      () => new Promise(() => {}),
    );

    render(
      <ChatUI
        accessToken="1234567890"
        token="1234567890"
        userRole="user"
        userID="1234567890"
        disabledPersonalKeyCreation={false}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Chat" })).toBeInTheDocument();
    });

    await user.click(screen.getByText(translate("en", "myModels.connectionSettings")));
    await user.click(screen.getByLabelText("Virtual Key Source"));
    await user.click(await screen.findByRole("option", { name: "Virtual Key" }));

    const keyField = await screen.findByPlaceholderText("Enter custom Virtual Key");
    fireEvent.change(keyField, { target: { value: "sk-test" } });

    await waitFor(() => {
      expect(screen.getByPlaceholderText("Loading models...")).toBeInTheDocument();
    });

    await user.clear(keyField);

    await waitFor(() => {
      expect(screen.getByPlaceholderText(translate("en", "Select a Model"))).toBeEnabled();
    });
  });
});
