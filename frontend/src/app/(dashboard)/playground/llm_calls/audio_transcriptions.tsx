import { postLLMRequest } from "@/components/llm_calls/transport";
import { getProxyBaseUrl } from "@/components/networking";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";

/** 调用媒体数据面并回调结果；参数包含输入、模型、密钥、标签及取消信号。
 * 返回完成 Promise，供 Playground 使用；HTTP/解析失败和取消向调用方传播。
 * 上传保留原文件，语音对象 URL 由消费组件释放。 */
export async function makeOpenAIAudioTranscriptionRequest(
  audioFile: File,
  updateUI: (transcription: string, model: string) => void,
  selectedModel: string,
  accessToken: string,
  tags?: string[],
  signal?: AbortSignal,
  language?: string,
  prompt?: string,
  responseFormat?: string,
  temperature?: number,
  customBaseUrl?: string,
) {
  // base url should be the current base_url
  const isLocal = process.env.NODE_ENV === "development";
  if (isLocal !== true) {
    console.log = function () {};
  }
  const proxyBaseUrl = customBaseUrl || getProxyBaseUrl();

  try {
    const form = new FormData();
    form.append("model", selectedModel);
    form.append("file", audioFile);
    if (language) form.append("language", language);
    if (prompt) form.append("prompt", prompt);
    if (responseFormat) form.append("response_format", responseFormat);
    if (temperature !== undefined) form.append("temperature", String(temperature));
    const httpResponse = await postLLMRequest("audio/transcriptions", form, {
      baseUrl: proxyBaseUrl,
      accessToken,
      tags,
      signal,
    });
    const response =
      responseFormat && !["json", "verbose_json"].includes(responseFormat)
        ? { text: await httpResponse.text() }
        : await httpResponse.json();

    // The response is a transcription object with a text field
    if (response && response.text) {
      updateUI(response.text, selectedModel);
      toast.success(t("Audio transcribed successfully"));
    } else {
      throw new Error(t("No transcription text in response"));
    }
  } catch (error: any) {
    console.error("Error making audio transcription request:", error);

    if (signal?.aborted) {
    } else {
      let errorMessage = "Failed to transcribe audio";

      if (error?.error?.message) {
        errorMessage = error.error.message;
      } else if (error?.message) {
        errorMessage = error.message;
      }

      toast.fromError(t("Audio transcription failed: {errorMessage}", { errorMessage }));
    }
    throw error; // Re-throw to allow the caller to handle the error
  }
}
