import { afterEach, describe, expect, it, vi } from "vitest";
import { makeOpenAIImageGenerationRequest } from "./image_generation";
import { makeOpenAIImageEditsRequest } from "./image_edits";
vi.mock("@/components/networking", () => ({ getProxyBaseUrl: () => "http://test" }));
afterEach(() => vi.unstubAllGlobals());
describe("图片 fetch 协议", () => {
  /** 前置 URL/内嵌图片响应；验证模型及回调结果，结束恢复 fetch，无持久化数据。 */
  it.each([{ url: "http://test/image.png" }, { b64_json: "aGVsbG8=" }])("生成图片保留结果 %j", async (image) => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: [image] })));
    vi.stubGlobal("fetch", fetch);
    const update = vi.fn();
    await makeOpenAIImageGenerationRequest("draw", update, "image", "key");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({ model: "image", prompt: "draw" });
    expect(update).toHaveBeenCalledWith(
      "url" in image ? image.url : "data:image/png;base64," + image.b64_json,
      "image",
    );
  });
  /** 前置无效或缺失数据；验证不会渲染伪成功，结束恢复 fetch。 */
  it.each([{}, { data: [] }, { data: [{}] }])("生成图片拒绝无效响应 %j", async (result) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(result))));
    const update = vi.fn();
    await expect(makeOpenAIImageGenerationRequest("draw", update, "image", "key")).rejects.toThrow();
    expect(update).not.toHaveBeenCalled();
  });
  /** 前置两张真实 File；验证分次表单上传、文件名与全部图片回调；结束恢复 fetch。 */
  it("编辑多图片保持上传文件与逐张结果", async () => {
    const fetch = vi
      .fn()
      .mockImplementation(async () => new Response(JSON.stringify({ data: [{ url: "http://test/edited" }] })));
    vi.stubGlobal("fetch", fetch);
    const files = [new File(["a"], "a.png"), new File(["b"], "b.png")];
    const update = vi.fn();
    await makeOpenAIImageEditsRequest(files, "edit", update, "image", "key");
    expect(fetch).toHaveBeenCalledTimes(2);
    for (let i = 0; i < 2; i++) {
      const options = fetch.mock.calls[i][1];
      expect(options.body.get("image").name).toBe(files[i].name);
      expect(options.body.get("prompt")).toBe("edit");
      expect(options.headers.has("content-type")).toBe(false);
    }
    expect(update).toHaveBeenCalledTimes(2);
  });
  /** 前置空列表与上游失败；验证空列表不请求、失败停止；恢复 fetch。 */
  it("编辑空输入不请求，HTTP 失败传播", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response('{"error":{"message":"bad image"}}', { status: 400 }));
    vi.stubGlobal("fetch", fetch);
    await makeOpenAIImageEditsRequest([], "edit", vi.fn(), "image", "key");
    expect(fetch).not.toHaveBeenCalled();
    await expect(
      makeOpenAIImageEditsRequest(new File(["a"], "a.png"), "edit", vi.fn(), "image", "key"),
    ).rejects.toThrow("bad image");
  });
});
