import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeOpenAIAudioTranscriptionRequest } from "./audio_transcriptions";
import { installLLMFetchFixture } from "../../../../../tests/llmFetchFixture";

describe("audio_transcription", () => {
  const mockCreate = vi.fn();
  const mockUpdateUI = vi.fn();
  let abortController: AbortController | null = null;

  beforeEach(() => {
    // Mock the response structure from OpenAI audio transcription API
    mockCreate.mockResolvedValue({
      text: "This is the transcribed text from the audio file.",
    });

    installLLMFetchFixture(mockCreate);
  });

  afterEach(() => {
    // Clean up abort controller if it exists
    if (abortController) {
      abortController.abort();
      abortController = null;
    }
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  /** 前置隔离协议响应；验证请求参数、业务回调或错误边界；afterEach 恢复 fetch 和 mock，无外部数据。 */
  it("should make a request to the audio transcription API with basic parameters", async () => {
    const mockFile = new File(["audio data"], "test.wav", {
      type: "audio/wav",
    });

    await makeOpenAIAudioTranscriptionRequest(mockFile, mockUpdateUI, "whisper-1", "sk-1234567890", []);

    expect(mockCreate).toHaveBeenCalledWith(
      {
        model: "whisper-1",
        file: mockFile,
      },
      { signal: undefined },
    );
    expect(mockUpdateUI).toHaveBeenCalledWith("This is the transcribed text from the audio file.", "whisper-1");
  });

  /** 前置隔离协议响应；验证请求参数、业务回调或错误边界；afterEach 恢复 fetch 和 mock，无外部数据。 */
  it("should include optional parameters when provided", async () => {
    const mockFile = new File(["audio data"], "test.mp3", {
      type: "audio/mpeg",
    });
    abortController = new AbortController();
    const signal = abortController.signal;

    await makeOpenAIAudioTranscriptionRequest(
      mockFile,
      mockUpdateUI,
      "whisper-1",
      "sk-1234567890",
      ["tag1", "tag2"],
      signal,
      "en",
      "This is a prompt",
      "json",
      0.5,
    );

    expect(mockCreate).toHaveBeenCalledWith(
      {
        model: "whisper-1",
        file: mockFile,
        language: "en",
        prompt: "This is a prompt",
        response_format: "json",
        temperature: 0.5,
      },
      { signal },
    );
    expect(mockUpdateUI).toHaveBeenCalledWith("This is the transcribed text from the audio file.", "whisper-1");
  });

  /** 前置隔离协议响应；验证请求参数、业务回调或错误边界；afterEach 恢复 fetch 和 mock，无外部数据。 */
  it("should handle errors gracefully", async () => {
    const mockError = new Error("API Error");
    mockCreate.mockRejectedValue(mockError);
    const mockFile = new File(["audio data"], "test.wav", {
      type: "audio/wav",
    });

    await expect(
      makeOpenAIAudioTranscriptionRequest(mockFile, mockUpdateUI, "whisper-1", "sk-1234567890", []),
    ).rejects.toThrow("API Error");

    expect(mockUpdateUI).not.toHaveBeenCalled();
  });

  /** 前置隔离协议响应；验证请求参数、业务回调或错误边界；afterEach 恢复 fetch 和 mock，无外部数据。 */
  it("should handle missing text in response", async () => {
    mockCreate.mockResolvedValue({});
    const mockFile = new File(["audio data"], "test.wav", {
      type: "audio/wav",
    });

    await expect(
      makeOpenAIAudioTranscriptionRequest(mockFile, mockUpdateUI, "whisper-1", "sk-1234567890", []),
    ).rejects.toThrow("No transcription text in response");

    expect(mockUpdateUI).not.toHaveBeenCalled();
  });
});

/** 前置非 JSON 字幕格式；验证原始文本回调，结束恢复 fetch，不产生外部数据。 */
it.each(["text", "srt", "vtt"])("转写保留 %s 文本响应", async (format) => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("transcript")));
  try {
    const update = vi.fn();
    await makeOpenAIAudioTranscriptionRequest(
      new File(["a"], "a.wav"),
      update,
      "whisper",
      "key",
      [],
      undefined,
      undefined,
      undefined,
      format,
    );
    expect(update).toHaveBeenCalledWith("transcript", "whisper");
  } finally {
    vi.unstubAllGlobals();
  }
});
