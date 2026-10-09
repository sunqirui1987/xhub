import { describe, it, expect } from "vitest";
import { priceCatalogRows } from "./priceCatalogRows";
import {
  marketSale,
  marketPrices,
  marketModalities,
  marketCapability,
  marketRetired,
  marketMoney,
  marketStrings,
  marketFeatures,
} from "./marketCatalog";
import type { EndpointDescriptor } from "@/components/add_model/endpointCatalog";
import { modelDeployments, modelCurl, marketDocUrl } from "./modelAccess";
/** 构造独立市场行，参数为原始字段，返回标准价格行；无网络与持久副作用。 */
const row = (fields: Record<string, unknown> = {}) =>
  priceCatalogRows({ models: [{ id: "test/model", market_catalog: true, ...fields }] })[0];
describe("市场目录及内部接入", () => {
  /** 前置能力标签与运营标签；验证合并去重以及缺失、非法字段降级；纯函数无需清理。 */
  it("特性筛选保留能力与运营标签", () => {
    expect(marketFeatures(row({ features: ["工具调用", "热门"], hot_tags: ["热门", "上新"] }))).toEqual([
      "工具调用",
      "热门",
      "上新",
    ]);
    expect(marketFeatures(row())).toEqual([]);
    expect(marketFeatures(row({ features: null, hot_tags: [null, 1, "视频生成"] }))).toEqual(["视频生成"]);
  });
  /** 正常公开模型可售；私有、退役、下架及撤回隐藏；输入独立且无需清理。 */
  it("只展示在售公开模型", () => {
    expect(marketSale(row())).toBe(true);
    for (const fields of [
      { private: true },
      { delisted: true },
      { feed_unavailable: true },
      { retirement_at: "2020-01-01" },
      { market_catalog: false },
      { removed: true },
    ])
      expect(marketSale(row(fields))).toBe(false);
    expect(marketRetired(row({ retirement_at: "invalid" }))).toBe(false);
    expect(marketRetired(row({ retirement_at: "2030-01-01" }), 0)).toBe(false);
  });
  /** 完整档位、单位换算、缺失和非法金额均验证；确定性数据，无清理。 */
  it("保留档位与缺失价格", () => {
    const prices = marketPrices(
      row({
        pricing_rules_v2: [
          {
            input_range: [0, 128000],
            details_v2: {
              input: { name: "输入", unit_name: "token", unit_size: 1000, unit_price_usd: 0.002 },
              video: { unit_name: "time", unit_price: 2 },
            },
          },
          { details_v2: { output: { unit_name: "token", unit_size: 1000000, unit_price_usd: -1 } } },
        ],
      }),
    );
    expect(prices).toHaveLength(3);
    expect(prices[0]).toMatchObject({ usd: 2, cny: null, unit: "1M tokens", tier: 0 });
    expect(prices[1].unit).toBe("1 次");
    expect(prices[2].usd).toBeNull();
    expect(
      marketPrices(
        row({
          pricing_rules_v2: [{ input_range: [null, 128000], details_v2: { input: { unit_price_usd: 1 } } }, null],
        }),
      )[0].range,
    ).toBe("");
    expect(marketPrices(row())).toEqual([]);
    expect(marketMoney(null)).toBe("未提供");
  });
  /** 能力只能取明确声明，坏输入返回空或false，无副作用。 */
  it("读取明确能力和模态", () => {
    const model = row({ input_modalities: ["text", "image"], architecture: { function_calling: { supported: true } } });
    expect(marketModalities(model, "input")).toEqual(["text", "image"]);
    expect(marketCapability(model, "function_calling")).toBe(true);
    expect(marketCapability(model, "reasoning")).toBe(false);
    expect(marketStrings([null, "text", 2])).toEqual(["text"]);
  });
  /** 已禁用部署仍算已配置但不生成调用示例；价格绑定优先，无外部调用。 */
  it("区分已配置和可调用部署", () => {
    const items = [
      { model_name: "internal", model_info: { base_model: "test/model", disabled: true } },
      { model_name: "other", model_info: { base_model: "other" }, litellm_params: { model: "test/model" } },
    ];
    expect(modelDeployments(row(), items)).toEqual([]);
    expect(modelDeployments(row(), items, true)).toHaveLength(1);
  });
  /** curl只使用登记接口和环境变量密钥，模型名安全转义；专用协议不给猜测示例，无副作用。 */
  it("生成安全内部示例并校验文档链接", () => {
    const deployment = { model_name: "name'quoted", model_info: { endpoint_types: ["chat"] } };
    const endpoint: EndpointDescriptor = {
      id: "chat",
      label: "Chat",
      category: "openai",
      paths: ["/v1/chat/completions"],
      kind: "adapted",
      protocol: "openai-chat",
      family: "openai",
    };
    expect(modelCurl(deployment, endpoint, "https://internal.test/")).toContain("$XHUB_API_KEY");
    expect(modelCurl(deployment, { ...endpoint, protocol: "video" }, "https://internal.test")).toBeNull();
    expect(marketDocUrl("javascript:alert(1)")).toBeNull();
    expect(marketDocUrl("https://docs.test")).toBe("https://docs.test/");
    expect(marketDocUrl(null)).toBeNull();
  });
});
