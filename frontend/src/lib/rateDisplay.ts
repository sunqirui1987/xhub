import { t } from "@/i18n";

/**
 * 一条费率的显示词汇。
 *
 * 这里只有一件事：**价目表和计费日志说同一套话**。
 *
 * 之前是两份实现：控制台的模型卡片自己拼一遍，日志详情又拼一遍。结果同一条
 * 费率在两处显示成不同的名字——一边是"输出 · 1080p_wiv_v"，另一边是英文的
 * "Output · uncached"。运维在两个页面上对不上账，因为两句"输出"指的不是同
 * 一档价。
 *
 * 所以侧、变体、维度、单位这四个词的翻译都收在这里。加一个新维度时改一处，
 * 两处一起变。
 */

/**
 * 一行费率的侧怎么说。
 * 参数 side（string）：input、output、cache_read 这些。
 * 返回 string（string）：界面上的名字。认不出的原样返回，宁可露出一个生词，也不要猜错。
 */
export function sideLabel(side: string | undefined): string {
  switch (side) {
    case "input":
      return t("Input");
    case "output":
      return t("Output");
    case "cache_read":
      return t("Cache read");
    case "cache_write":
      return t("Cache write");
    case "batch_input":
      return t("Batch input");
    case "batch_output":
      return t("Batch output");
    case "":
    case undefined:
      return "";
    default:
      return side;
  }
}

/** 变体词表：市场键名里那段限定词到界面用词。 */
const VARIANT_WORDS: Record<string, string> = {
  uncached: "Uncached",
  cached: "Cache hit",
  nth: "Non-thinking",
  th: "Thinking",
  non_thinking: "Non-thinking",
  thinking: "Thinking",
  wiv: "With video input",
  woiv: "Without video input",
  t2v: "Text to video",
  i2v: "Image to video",
  r2v: "Reference to video",
  av: "With audio",
  ti: "Text to image",
  ii: "Image to image",
  mi2i: "Multi-image to image",
  omi: "Single image",
  text: "Text",
  image: "Image",
  audio: "Audio",
  search: "Web search",
};

/** 分辨率这类原样就看得懂的片段。 */
const PASSTHROUGH = /^(\d+k|\d+p|\d+_\d+k|\d+_?\d*p)$/;

/**
 * 一段变体怎么说。
 *
 * 变体原文是市场键名里剩下的那段（`1080p_wiv_v`、`non_thinking`），**不能
 * 直接摆到界面上**——那是键名，不是给运维读的字。这里把认得的片段翻成词，
 * 分辨率和认不出的原样留着。
 *
 * 参数 variant（string）：价目表或快照里的变体字段。
 * 返回 string（string）：可以直接显示的描述。没有变体时是空串。
 */
export function variantLabel(variant: string | undefined): string {
  if (!variant) return "";
  // 整体命中时优先整体翻：non_thinking 拆开之后两个词各自都没有意义。
  const whole = VARIANT_WORDS[variant];
  if (whole) return t(whole);
  const parts = variant
    .split("_")
    .map((part) => part.trim())
    .filter(Boolean)
    // 键名里那个 `v` 是 "video" 的缩写，是分隔符不是内容。
    .filter((part) => part !== "v");
  if (parts.length === 0) return "";
  return parts
    .map((part) => {
      const word = VARIANT_WORDS[part];
      if (word) return t(word);
      if (PASSTHROUGH.test(part)) return part.toUpperCase();
      return part;
    })
    .join(" · ");
}

/**
 * 一条费率整行怎么说：侧加变体。
 * 参数 side（string）：哪一侧；variant（string）：同侧内部的限定词。
 * 返回 string（string）：例如 "Output · With video input" 或 "Cache read"。
 */
export function rateLabel(side: string | undefined, variant: string | undefined): string {
  const base = sideLabel(side);
  const detail = variantLabel(variant);
  if (!base) return detail;
  return detail ? `${base} · ${detail}` : base;
}

/**
 * 一个计费维度怎么说。token、picture、second、query。
 * 参数 measure（string）：价目表里的 measure。
 * 返回 string（string）：界面上的名字。认不出的原样返回。
 */
export function measureLabel(measure: string | undefined): string {
  switch (measure) {
    case "token":
      return t("tokens");
    case "picture":
      return t("pictures");
    case "second":
      return t("seconds");
    case "query":
      return t("queries");
    case "":
    case undefined:
      return "";
    default:
      return measure;
  }
}

/**
 * 一个时段怎么说。peak、offpeak，或者不分时段。
 * 参数 window（string）：价目表或快照里的 window。
 * 返回 string（string）：界面上的名字。
 */
export function windowLabel(window: string | undefined): string {
  switch (window) {
    case "peak":
      return t("Peak");
    case "offpeak":
      return t("Off-peak");
    default:
      return t("Any hour");
  }
}

/**
 * 一档单价怎么显示。
 *
 * token 按每百万显示，那是运维习惯的单位；按秒、按张、按次的原样显示。
 * 判断走 measure，不假设所有模型都按 token 计费——把一张图的价写成每百万
 * 会显示成一个荒谬的小数。
 *
 * 参数 usd（number）：每一个基础单位的美元价；measure（string）：计费维度。
 * 返回 string（string）：可以直接显示的单价。
 */
export function rateText(usd: number | undefined, measure: string | undefined): string {
  if (usd === undefined || usd === null) return "-";
  if (measure !== "token") {
    return "$" + new Intl.NumberFormat("en-US", { maximumSignificantDigits: 6 }).format(usd);
  }
  return "$" + new Intl.NumberFormat("en-US", { maximumSignificantDigits: 6 }).format(usd * 1_000_000) + " /1M";
}
