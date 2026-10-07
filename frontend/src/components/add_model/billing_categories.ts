import { t } from "@/i18n";

/**
 * 单价的计费分类。
 *
 * 这一份是纯数据加两个查表函数，不依赖 React，所以可以直接单测。组件
 * （model_price_fields.tsx）只负责把它渲染成格子。
 *
 * 分类为什么重要：网关按**模型名**取单价，取不到就记一行零费用，不报错也不
 * 告警。表单显示错一组格子，运维会填了不读的那个（以为定过价，实际记零），
 * 或者看不到该填的那个。所以"哪种调用方式显示哪些格子"是值得钉住的逻辑，
 * 不是排版细节。
 */

/** 计费维度。一条模型通常只按其中一种计费。 */
export type BillingMode = "token" | "image" | "second";

/** 时段。只有带分时价的模型才会用到 peak。 */
export type Window = "offpeak" | "peak" | "all";

/** 一个费率字段：表单名、标签、提示、单位换算，以及怎么从价目表带出默认值。 */
export type RateField = {
  name: string;
  label: string;
  hint?: string;
  /** 界面单位换算到存储单位的倍数。token 类是 1e6，其余是 1。 */
  scale: number;
  /** 目录里的同名字段。取不到时再看 ratesFilter。 */
  catalogKey: string;
  /** 从目录的 rates[] 里按 side+window 找值的条件。 */
  ratesFilter?: { side: string; window: string };
};

/** 一节费率：同一个计费维度下时段相同的一组格子。 */
export type BillingCategory = {
  id: string;
  /** 这一节适用于哪几种计费维度。 */
  modes: BillingMode[];
  /** 时段。带 _peak 后缀的格子靠这个值区分。 */
  window: Window;
  title: string;
  unit: string;
  fields: RateField[];
};

const OFFPEAK_WINDOW_HINT = t(
  "USD per 1M tokens · Mon–Fri outside 09:00–12:00 & 14:00–18:00 Beijing time, plus weekends and holidays",
);
const PEAK_WINDOW_HINT = t(
  "USD per 1M tokens · Mon–Fri 09:00–12:00 and 14:00–18:00 Beijing time (excl. public holidays)",
);

const ALL_MODES: BillingMode[] = ["token", "image", "second"];

/**
 * 全部计费分类。顺序就是表单上的显示顺序。
 *
 * 缓存那两档排在时段之后：缓存读写的价和时段无关，混在空闲/高峰之间会让人
 * 以为缓存也分时。
 */
export const BILLING_CATEGORIES: BillingCategory[] = [
  {
    id: "token_offpeak",
    modes: ["token"],
    window: "offpeak",
    title: t("Off-peak"),
    unit: OFFPEAK_WINDOW_HINT,
    fields: [
      {
        name: "input_cost_per_token",
        label: t("Input"),
        scale: 1e6,
        catalogKey: "input_cost_per_token",
        ratesFilter: { side: "input", window: "offpeak" },
      },
      {
        name: "output_cost_per_token",
        label: t("Output"),
        scale: 1e6,
        catalogKey: "output_cost_per_token",
        ratesFilter: { side: "output", window: "offpeak" },
      },
    ],
  },
  {
    id: "token_peak",
    modes: ["token"],
    window: "peak",
    title: t("Peak"),
    unit: PEAK_WINDOW_HINT,
    fields: [
      {
        name: "input_cost_per_token_peak",
        label: t("Input (peak)"),
        scale: 1e6,
        catalogKey: "input_cost_per_token_peak",
        ratesFilter: { side: "input", window: "peak" },
      },
      {
        name: "output_cost_per_token_peak",
        label: t("Output (peak)"),
        scale: 1e6,
        catalogKey: "output_cost_per_token_peak",
        ratesFilter: { side: "output", window: "peak" },
      },
    ],
  },
  {
    id: "token_cache",
    modes: ["token"],
    window: "all",
    title: t("Cache"),
    unit: t("USD per 1M tokens"),
    fields: [
      {
        name: "cache_read_input_token_cost",
        label: t("Cache read"),
        hint: t("What a cached prompt costs. Usually far below the input rate."),
        scale: 1e6,
        catalogKey: "cache_read_input_token_cost",
        ratesFilter: { side: "cache_read", window: "all" },
      },
      {
        name: "cache_creation_input_token_cost",
        label: t("Cache write"),
        hint: t("What writing the cache costs. Usually above the input rate."),
        scale: 1e6,
        catalogKey: "cache_creation_input_token_cost",
        ratesFilter: { side: "cache_write", window: "all" },
      },
    ],
  },
  {
    id: "per_image",
    modes: ["image"],
    window: "all",
    title: t("Per image"),
    unit: t("USD per image"),
    fields: [
      {
        name: "input_cost_per_image",
        label: t("Input image"),
        scale: 1,
        catalogKey: "input_cost_per_image",
        ratesFilter: { side: "input", window: "all" },
      },
      {
        name: "output_cost_per_image",
        label: t("Output image"),
        scale: 1,
        catalogKey: "output_cost_per_image",
        ratesFilter: { side: "output", window: "all" },
      },
    ],
  },
  {
    id: "per_second",
    modes: ["second"],
    window: "all",
    title: t("Per second"),
    unit: t("USD per second"),
    fields: [
      {
        name: "input_cost_per_second",
        label: t("Input per second"),
        scale: 1,
        catalogKey: "input_cost_per_second",
        ratesFilter: { side: "input", window: "all" },
      },
      {
        name: "output_cost_per_second",
        label: t("Output per second"),
        scale: 1,
        catalogKey: "output_cost_per_second",
        ratesFilter: { side: "output", window: "all" },
      },
    ],
  },
  {
    id: "per_query",
    modes: ALL_MODES,
    window: "all",
    title: t("Search"),
    unit: t("USD per search query"),
    fields: [
      {
        name: "search_context_cost_per_query",
        label: t("Per search"),
        scale: 1,
        catalogKey: "search_context_cost_per_query",
        ratesFilter: { side: "output", window: "all" },
      },
    ],
  },
];

/**
 * 调用方式到计费维度的对应。
 *
 * 键是能力 id，取值来自 /public/endpoints 的能力表。没有列出的能力按 token
 * 计费：那是绝大多数模型，也是网关默认读的那组字段。
 *
 * 参数 capability（string | undefined）：当前选中的调用方式 id。
 * 返回 BillingMode（BillingMode）：按哪种维度计费。
 */
export function categoryOf(capability: string | undefined): BillingMode {
  switch (capability) {
    case "image":
      return "image";
    case "video":
    case "audio_speech":
      return "second";
    default:
      return "token";
  }
}

/**
 * 这个调用方式要显示哪几节。
 * 参数 capability（string | undefined）：当前选中的调用方式 id。
 * 返回 BillingCategory[]（BillingCategory[]）：适用的分类，顺序与定义一致。
 */
export function categoriesOf(capability: string | undefined): BillingCategory[] {
  const mode = categoryOf(capability);
  return BILLING_CATEGORIES.filter((category) => category.modes.includes(mode));
}

/**
 * 这个调用方式要显示哪些费率格子。
 * 参数 capability（string | undefined）：当前选中的调用方式 id。
 * 返回 string[]（string[]）：表单字段名，按显示顺序。
 */
export function rateFieldsOf(capability: string | undefined): string[] {
  return categoriesOf(capability).flatMap((category) => category.fields.map((field) => field.name));
}
