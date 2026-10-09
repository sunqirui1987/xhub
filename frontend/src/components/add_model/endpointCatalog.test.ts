import { describe, it, expect } from "vitest";
import { catalogTransports, defaultTransport, compatibleEndpoint, protocolLabel, defaultEndpoints, publicEndpointGroup, type EndpointDescriptor, type EndpointTransport } from "./endpointCatalog";
const generation: EndpointDescriptor = {id:"image_generation", label:"创建", category:"openai", kind:"bypass", protocol:"openai-images", family:"image"};
const transport: EndpointTransport = {id:"openai_image_generation", label:"创建", kind:"bypass", endpoint_id:"image_generation", protocol:"openai-images"};
describe("端点与执行目录", () => {
  /** 前置静态目录；验证生图不能误选改图、未知或空传输；无持久数据及清理。 */
  it("按具体图片操作而非共享协议匹配", () => {
    expect(compatibleEndpoint(generation, transport)).toBe(true);
    expect(compatibleEndpoint({...generation,id:"image_edit"},transport)).toBe(false);
    expect(compatibleEndpoint({...generation,id:"image"},transport)).toBe(false);
    expect(compatibleEndpoint(generation)).toBe(false);
  });
  /** 前置三种已实现对话协议；接入可转换，Bypass 必须相同原厂协议；无需清理。 */
  it("区分对话转换与原生透传", () => {
    const chat = {...generation,id:"chat",kind:"adapted" as const,protocol:"openai-chat",family:"chat"};
    const messages = {...transport,endpoint_id:"bypass:anthropic-messages",protocol:"anthropic-messages"};
    expect(compatibleEndpoint(chat,messages)).toBe(true);
    expect(compatibleEndpoint({...chat,id:"bypass:openai-chat",kind:"bypass"},messages)).toBe(false);
    expect(compatibleEndpoint({...chat,id:"gemini",kind:"bypass",protocol:"gemini"},messages)).toBe(false);
  });
  /** 前置图片与未知协议；验证上游选择框区分创建/编辑并保留自定义组标签；无需清理。 */
  it("使用原厂协议标签", () => {
    expect(protocolLabel(transport)).toBe("OpenAI Images · 创建图片");
    expect(protocolLabel({...transport,endpoint_id:"image_edit"})).toBe("OpenAI Images · 编辑图片");
    expect(protocolLabel({...transport,endpoint_id:"bypass:openai-images"})).toBe("OpenAI Images · 创建图片 · Bypass");
    expect(protocolLabel({...transport,endpoint_id:"bypass:openai-image-edit"})).toBe("OpenAI Images · 编辑图片 · Bypass");
    expect(protocolLabel({...transport,protocol:"unknown",model_group:"供应商队列"})).toBe("供应商队列");
  });
});

/** 前置三协议转换目录；验证标准接口自动开放、原生 Bypass 需显式选择以及空配置无兜底，无数据清理。 */
it("从上游能力生成固定接口并将 Fal 归入 Bypass", () => {
 const entries: EndpointDescriptor[] = [
  {id:"chat",label:"Chat",category:"openai",kind:"adapted",protocol:"openai-chat",family:"chat"},
  {id:"responses",label:"Responses",category:"openai",kind:"adapted",protocol:"openai-responses",family:"chat"},
  {id:"messages",label:"Messages",category:"claude",kind:"adapted",protocol:"anthropic-messages",family:"chat"},
  {id:"bypass:openai-chat",label:"Bypass",category:"bypass",kind:"bypass",protocol:"openai-chat",family:"chat"},
 ];
 expect(defaultEndpoints(entries,{...transport,endpoint_id:"bypass:openai-chat",protocol:"openai-chat"})).toEqual(["chat","responses","messages"]);
 expect(defaultEndpoints(entries)).toEqual([]);
 expect(publicEndpointGroup({...generation,id:"fal:queue",protocol:"fal",category:"bypass"})).toBe("bypass");
 expect(defaultEndpoints([generation],transport)).toEqual(["image_generation"]);
});

/** 前置供应商模型映射和通用协议；验证通用协议保留、专用型号匹配、别名前缀、未知/错目录和历史空目录；无副作用。 */
it("keeps generic protocols while validating dedicated catalog registrations", () => {
 const ark={...transport,id:"ark",catalog_id:"qiniu",protocol:"ark"};
 const fal={...transport,id:"fal",catalog_id:"qiniu",protocol:"fal"};
 const catalogs={qiniu:{seedance:["ark"],"fal/model":["fal"]}};
 const all=[transport,ark,fal];
 expect(catalogTransports(all,catalogs,"qiniu","seedance","custom")).toEqual([transport,ark]);
 expect(catalogTransports(all,catalogs,"qiniu","qiniu/fal/model","custom")).toEqual([transport,fal]);
 expect(catalogTransports(all,catalogs,"unknown","seedance","custom")).toEqual(all);
 expect(catalogTransports(all,catalogs,"qiniu","unknown","custom")).toEqual([transport]);
 expect(catalogTransports(all,catalogs,"qiniu","seedance","custom")).toContain(transport);
 expect(catalogTransports(all,catalogs,"","seedance","custom")).toEqual(all);
 expect(catalogTransports([{...ark,catalog_id:"other"}],catalogs,"qiniu","seedance","custom")).toEqual([]);
});

/** 前置真实七牛执行字段；验证 FAL 与 Ark 标签可辨、OpenAI 连接可选择、未知模型和错误连接不被视为支持；纯函数无清理。 */
it("shows FAL explicitly and filters the exact task path for OpenAI connections", () => {
 const fal: EndpointTransport = {...transport,id:"qiniu_fal_dreamina_20",catalog_id:"qiniu",protocol:"fal",endpoint_id:"fal:queue",model_group:"Dreamina Seedance 2.0",providers:["custom","custom_openai","openai"],strip_prefix:"qiniu",actions:[{name:"create",public_path:"/queue/byteplus/seedance-2.0/text-to-video",model:"byteplus/seedance-2.0/text-to-video"}]};
 const catalogs={qiniu:{"byteplus/seedance-2.0/text-to-video":[fal.id]}};
 expect(protocolLabel(fal)).toBe("FAL · Dreamina Seedance 2.0");
 expect(protocolLabel({...fal,protocol:"ark",model_group:"Seedance"})).toBe("Ark Video · Seedance");
 expect(catalogTransports([fal],catalogs,"qiniu","byteplus/seedance-2.0/text-to-video","openai")).toEqual([fal]);
 expect(catalogTransports([fal],catalogs,"qiniu","qiniu/byteplus/seedance-2.0/text-to-video","openai")).toEqual([fal]);
 expect(catalogTransports([fal],catalogs,"qiniu","bytedance/doubao-seedance-2-0-260128","openai")).toEqual([]);
 expect(catalogTransports([fal],catalogs,"qiniu","byteplus/seedance-2.0/text-to-video","anthropic")).toEqual([]);
 expect(catalogTransports([fal],catalogs,"fennoai","byteplus/seedance-2.0/text-to-video","openai")).toEqual([fal]);
 expect(catalogTransports([fal],catalogs,"fennoai","unknown/video","openai")).toEqual([]);
});

/** 前置常规协议与专用协议；验证按连接默认、专用型号优先、未知连接及空目录边界；纯函数无清理。 */
it("defaults regular protocols from the connection without blocking manual choices", () => {
 const all: EndpointTransport[] = [
 {...transport,id:"chat",protocol:"openai-chat"}, {...transport,id:"vertex",protocol:"vertex"},
 {...transport,id:"gemini",protocol:"gemini"}, {...transport,id:"claude",protocol:"anthropic-messages"},
 {...transport,id:"fal",protocol:"fal",catalog_id:"qiniu"}];
 expect(defaultTransport(all,"openai")).toBe("chat");
 expect(defaultTransport(all,"custom_openai")).toBe("chat");
 expect(defaultTransport(all,"vertex_ai")).toBe("vertex");
 expect(defaultTransport(all,"gemini")).toBe("gemini");
 expect(defaultTransport(all,"anthropic")).toBe("claude");
 expect(defaultTransport(all,"openai","fal")).toBe("fal");
 expect(defaultTransport(all,"unknown")).toBe("");
 expect(defaultTransport([],"openai")).toBe("");
});
