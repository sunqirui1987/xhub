import { postLLMRequest } from "@/components/llm_calls/transport";
import { getProxyBaseUrl } from "@/components/networking";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";

/** 调用媒体数据面并回调结果；参数包含输入、模型、密钥、标签及取消信号。
 * 返回完成 Promise，供 Playground 使用；HTTP/解析失败和取消向调用方传播。
 * 上传保留原文件，语音对象 URL 由消费组件释放。 */
export async function makeOpenAIImageGenerationRequest(
  prompt: string,
  updateUI: (imageUrl: string, model: string) => void,
  selectedModel: string,
  accessToken: string,
  tags?: string[],
  signal?: AbortSignal,
  customBaseUrl?: string,
) {
  // base url should be the current base_url
  const isLocal = process.env.NODE_ENV === "development";
  if (isLocal !== true) {
    console.log = function () {};
  }
  const proxyBaseUrl = customBaseUrl || getProxyBaseUrl();

  try {
    const httpResponse = await postLLMRequest(
      "images/generations",
      { model: selectedModel, prompt },
      {
        baseUrl: proxyBaseUrl,
        accessToken,
        tags,
        signal,
      },
    );
    const response = await httpResponse.json();

    if (response.data && response.data[0]) {
      // Handle either URL or base64 data from response
      if (response.data[0].url) {
        // Use the URL directly
        updateUI(response.data[0].url, selectedModel);
      } else if (response.data[0].b64_json) {
        // Convert base64 to data URL format
        const base64Data = response.data[0].b64_json;
        updateUI(`data:image/png;base64,${base64Data}`, selectedModel);
      } else {
        throw new Error(t("No image data found in response"));
      }
    } else {
      throw new Error(t("Invalid response format"));
    }
  } catch (error) {
    if (signal?.aborted) {
    } else {
      toast.fromError(t("Error occurred while generating image. Please try again. Error: {error}", { error }));
    }
    throw error; // Re-throw to allow the caller to handle the error
  }
}
