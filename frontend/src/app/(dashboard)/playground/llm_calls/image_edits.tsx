import { postLLMRequest } from "@/components/llm_calls/transport";
import { getProxyBaseUrl } from "@/components/networking";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";

/** 调用媒体数据面并回调结果；参数包含输入、模型、密钥、标签及取消信号。
 * 返回完成 Promise，供 Playground 使用；HTTP/解析失败和取消向调用方传播。
 * 上传保留原文件，语音对象 URL 由消费组件释放。 */
export async function makeOpenAIImageEditsRequest(
  imageFiles: File | File[],
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
    // handle single and multiple images
    const imagesToProcess = Array.isArray(imageFiles) ? imageFiles : [imageFiles];

    // For multiple images, we'll make separate API calls for each image
    // since OpenAI's edit endpoint processes one image at a time
    const results = [];

    for (let i = 0; i < imagesToProcess.length; i++) {
      const image = imagesToProcess[i];

      const form = new FormData();
      form.append("model", selectedModel);
      form.append("image", image);
      form.append("prompt", prompt);
      const httpResponse = await postLLMRequest("images/edits", form, {
        baseUrl: proxyBaseUrl,
        accessToken,
        tags,
        signal,
      });
      const response = await httpResponse.json();

      if (response.data && response.data[0]) {
        // Handle either URL or base64 data from response
        if (response.data[0].url) {
          // Use the URL directly
          updateUI(response.data[0].url, selectedModel);
          results.push(response.data[0].url);
        } else if (response.data[0].b64_json) {
          // Convert base64 to data URL format
          const base64Data = response.data[0].b64_json;
          const dataUrl = `data:image/png;base64,${base64Data}`;
          updateUI(dataUrl, selectedModel);
          results.push(dataUrl);
        }
      }
    }

    if (results.length > 1) {
      toast.success(t("Successfully processed {value0} images", { value0: results.length }));
    }
  } catch (error: any) {
    console.error("Error making image edit request:", error);

    if (signal?.aborted) {
    } else {
      let errorMessage = "Failed to edit image(s)";

      if (error?.error?.message) {
        errorMessage = error.error.message;
      } else if (error?.message) {
        errorMessage = error.message;
      }

      toast.fromError(t("Image edit failed: {errorMessage}", { errorMessage }));
    }
    throw error; // Re-throw to allow the caller to handle the error
  }
}
