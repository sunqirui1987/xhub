import { postLLMRequest } from "@/components/llm_calls/transport";
import { getProxyBaseUrl } from "@/components/networking";
import { toast } from "@/lib/toast";
import type { OpenAIVoice } from "../components/chat_ui/chatConstants";
import { t } from "@/i18n";

/** 调用媒体数据面并回调结果；参数包含输入、模型、密钥、标签及取消信号。
 * 返回完成 Promise，供 Playground 使用；HTTP/解析失败和取消向调用方传播。
 * 上传保留原文件，语音对象 URL 由消费组件释放。 */
export async function makeOpenAIAudioSpeechRequest(
  input: string,
  voice: OpenAIVoice,
  updateUI: (audioUrl: string, model: string) => void,
  selectedModel: string,
  accessToken: string,
  tags?: string[],
  signal?: AbortSignal,
  responseFormat?: string,
  speed?: number,
  customBaseUrl?: string,
) {
  // base url should be the current base_url
  const isLocal = process.env.NODE_ENV === "development";
  if (isLocal !== true) {
    console.log = function () {};
  }
  const proxyBaseUrl = customBaseUrl || getProxyBaseUrl();

  try {
    const response = await postLLMRequest(
      "audio/speech",
      {
        model: selectedModel,
        input,
        voice,
        ...(responseFormat ? { response_format: responseFormat } : {}),
        ...(speed !== undefined ? { speed } : {}),
      },
      { baseUrl: proxyBaseUrl, accessToken, tags, signal },
    );

    // Convert the response to a blob and create an object URL
    // The response from OpenAI SDK in the browser is a Response object with a blob() method
    const blob = await response.blob();
    const audioUrl = URL.createObjectURL(blob);

    updateUI(audioUrl, selectedModel);
  } catch (error) {
    if (signal?.aborted) {
    } else {
      toast.fromError(t("Error occurred while generating speech. Please try again. Error: {error}", { error }));
    }
    throw error; // Re-throw to allow the caller to handle the error
  }
}
