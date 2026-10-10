import { expect, it } from "vitest";
import { imagePreviewSources, isOpenAIImageEndpoint, parseNativeRequest, validateImageRequest } from "./imageRequest";
import { nativeRequestTemplate } from "./nativeRequestTemplate";
import type { ModelEndpoint } from "@/components/llm_calls/fetch_models";

const endpoint: ModelEndpoint = {
  endpoint_id: "images",
  kind: "bypass",
  transport: "bypass_openai_image_generation",
  protocol: "openai-images",
  family: "image",
  method: "POST",
  path: "/bypass/openai/v1/images/generations",
};

/** 前置创建/编辑及未知协议绑定；验证只为 OpenAI 图片补齐尺寸和数量，不污染其他供应商；纯函数无清理。 */
it("已声明的创建与编辑图片提供显式尺寸默认值", () => {
  for (const path of [endpoint.path, "/v1/images/edits"]) {
    expect(isOpenAIImageEndpoint({ ...endpoint, path })).toBe(true);
    expect(JSON.parse(nativeRequestTemplate({ ...endpoint, path }))).toEqual({ prompt: "", size: "1024x1024", n: 1 });
  }
  expect(isOpenAIImageEndpoint({ ...endpoint, protocol: "fal" })).toBe(false);
  expect(isOpenAIImageEndpoint({ ...endpoint, path: "/unknown" })).toBe(false);
});

/** 前置对象、空对象和非法草稿；验证正常解析与错误边界，扩展字段完整；纯函数无副作用。 */
it("JSON 必须为对象且保留扩展字段", () => {
  expect(parseNativeRequest('{"seed":0}')).toEqual({ seed: 0 });
  expect(parseNativeRequest("{}")).toEqual({});
  for (const value of ["[]", "null", "1", '"text"', "{bad"]) expect(() => parseNativeRequest(value)).toThrow();
});

/** 前置合法、自定义和 auto 尺寸；验证不限定渠道能力，数量允许省略且正整数可提交；无清理。 */
it.each(["1024x1024", "1536x1024", "1024x1536", "2048x2048", "auto"])("接受有效尺寸 %s", (size) => {
  expect(() => validateImageRequest({ prompt: "猫", size, n: 1 })).not.toThrow();
  expect(() => validateImageRequest({ prompt: "猫", size })).not.toThrow();
});

/** 前置缺失、空白、类型错误或非正尺寸；验证提交前拒绝参数，不访问网络；无清理。 */
it.each([undefined, "", " ", "1024", "0x1024", "-1x1024", "1024X1024", 1024])("拒绝非法尺寸 %s", (size) => {
  expect(() => validateImageRequest({ prompt: "猫", size })).toThrow("图片尺寸");
});

/** 前置空提示词及数量边界；验证失败信息定位对应业务字段；纯函数无清理。 */
it("拒绝空提示词和非法数量", () => {
  for (const prompt of [undefined, "", "  ", 1])
    expect(() => validateImageRequest({ prompt, size: "auto" })).toThrow("提示词");
  for (const n of [0, -1, 1.5, "1", "", NaN, Infinity])
    expect(() => validateImageRequest({ prompt: "猫", size: "auto", n })).toThrow("正整数");
});

/** 前置 URL、base64 及恶意/异常响应；验证格式及安全过滤，无网络或数据清理。 */
it("提取安全预览并忽略非图片及错误响应", () => {
  expect(
    imagePreviewSources(
      JSON.stringify({
        data: [{ b64_json: "AAAA" }, { url: "https://example.com/image.png" }, { url: "javascript:alert(1)" }, null],
      }),
      "jpeg",
    ),
  ).toEqual(["data:image/jpeg;base64,AAAA", "https://example.com/image.png"]);
  expect(imagePreviewSources('{"data":[{"b64_json":"AAAA"}]}', "webp")).toEqual(["data:image/webp;base64,AAAA"]);
  for (const response of ["bad", "null", "{}", '{"data":{}}', '{"data":[{},1,{"url":"bad"},{"b64_json":"<svg>"}]}'])
    expect(imagePreviewSources(response)).toEqual([]);
});
