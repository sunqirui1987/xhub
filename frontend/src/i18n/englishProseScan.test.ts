import { describe, expect, it } from "vitest";
import { findHitsInSource, isEnglishProse } from "./englishProseScan";

describe("englishProseScan", () => {
  // 前提：候选文本可能混有中文及技术标识符；结果：只排除可证明的技术词，并检出普通英文；本测试无外部数据，无需清理。
  it("distinguishes Chinese technical copy from untranslated English prose", () => {
    expect(isEnglishProse("发送 JSON POST，timeout 单位为秒，上限 10 秒。")).toBe(false);
    expect(isEnglishProse("使用 XHub API Key，并设置 XHUB_API_KEY 环境变量。")).toBe(false);
    expect(isEnglishProse("Failed to load provider configuration")).toBe(true);
    expect(isEnglishProse("No media response data available")).toBe(true);
    expect(isEnglishProse("e.g. sensitive-data", { singleWordUi: true })).toBe(false);
    expect(isEnglishProse("pricing", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("search", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("This setting is required 中文", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("Please retry 中文", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("PLEASE RETRY 中文", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("SAVE 中文", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("SAVE", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("PLEASE RETRY", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("SYSTEM", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("USER", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("ASSISTANT", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("XHUB_API_KEY", { singleWordUi: true })).toBe(false);
    expect(isEnglishProse("设置 XHUB_API_KEY 环境变量", { singleWordUi: true })).toBe(false);
    expect(isEnglishProse("Headers", { singleWordUi: true })).toBe(true);
    expect(isEnglishProse("{value0} · USD", { singleWordUi: true })).toBe(false);
  });

  // 前提：JSX 同时包含中文技术说明和英文展示文案；结果：保留真实英文命中并排除技术词；纯内存扫描无需清理。
  it("keeps genuine JSX prose detections after filtering mixed Chinese copy", () => {
    const source = `
      export function Panel() {
        return <section description="调用 AWS API，失败时返回 error。">
          <p>Missing endpoint catalog</p>
          <input placeholder="Search models by name" />
        </section>;
      }
    `;
    expect(findHitsInSource("Panel.tsx", source).map((hit) => hit.template)).toEqual([
      "Missing endpoint catalog",
      "Search models by name",
    ]);
  });

  // 前提：翻译函数首参含条件式或拼接键；结果：目录键不报错、相邻未翻译提示仍命中；纯源码样本无需清理。
  it("recognizes composed translation keys without suppressing adjacent prose", () => {
    const source = `
      const label = t(active ? "Save model provider" : "Add model provider");
      const tab = t("myModels." + value);
      toast.error("Save model provider before continuing");
    `;
    expect(findHitsInSource("keys.tsx", source).map((hit) => hit.template)).toEqual([
      "Save model provider before continuing",
    ]);
  });

  // 前提：外部护栏固定目录与普通对象含相同英文；结果：仅双语 helper 消费的目录被识别；纯源码样本无需清理。
  it("recognizes only the external guardrail catalog consumed by the bilingual helper", () => {
    const catalog = `
      const EXTERNAL_PROVIDERS = { demo: { description: "Calls provider API", fields: [{ label: "API version" }] } };
    `;
    const ordinary = `const panel = { label: "API version", description: "Calls provider API" };`;
    expect(findHitsInSource("externalProviders.ts", catalog)).toEqual([]);
    expect(findHitsInSource("Panel.tsx", ordinary).map((hit) => hit.template)).toEqual([
      "API version",
      "Calls provider API",
    ]);
  });

  // 前提：小写分支键既可能由翻译函数消费，也可能被直接展示；结果：只跳过前者，直接可见的 pricing/search 仍命中；纯源码扫描无需清理。
  it("distinguishes translated map keys from directly rendered lowercase labels", () => {
    const translated = `["pricing", "search"].map((value) => <span>{t("tabs." + value)}</span>);`;
    const visible = `["pricing", "search"].map((value) => <span>{value}</span>);`;
    expect(findHitsInSource("Translated.tsx", translated)).toEqual([]);
    expect(findHitsInSource("Visible.tsx", visible).map((hit) => hit.template)).toEqual(["pricing", "search"]);
  });

  // 前提：中文句子可含表达式与明确技术词；结果：表达式占位不会伪造成英文，夹带自然英文的句子仍报告；纯源码扫描无需清理。
  it("does not let interpolation hide mixed-language prose or create false English", () => {
    const source = `
      const ok = <p>已获取 {count} 个模型，可输入模型 ID。</p>;
      const bad = <p>请求失败，请 Please retry {count} 次。</p>;
    `;
    expect(findHitsInSource("Mixed.tsx", source).map((hit) => hit.template)).toEqual([
      "请求失败，请 Please retry {count}次。",
    ]);
  });
});
