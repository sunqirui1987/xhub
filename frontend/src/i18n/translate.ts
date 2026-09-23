import { scrubBrand } from "./brand";
import { en } from "./messages/en";
import { zhCN } from "./messages/zh-CN";

export const locales = ["zh-CN", "en"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "zh-CN";
export const LOCALE_COOKIE = "xhub_locale";

export const catalogs: Record<Locale, unknown> = {
  "zh-CN": zhCN,
  en,
};

export type TranslateVars = Record<string, unknown>;

export function isLocale(value: string | null | undefined): value is Locale {
  return value === "zh-CN" || value === "en";
}

export function parseLocale(value: string | null | undefined): Locale {
  return isLocale(value) ? value : defaultLocale;
}

function phraseBook(tree: unknown): Record<string, unknown> | undefined {
  if (tree == null || typeof tree !== "object" || !("phrases" in tree)) return undefined;
  const phrases = (tree as { phrases?: unknown }).phrases;
  if (phrases == null || typeof phrases !== "object") return undefined;
  return phrases as Record<string, unknown>;
}

function lookup(tree: unknown, key: string): string | undefined {
  const phrases = phraseBook(tree);
  if (phrases && Object.prototype.hasOwnProperty.call(phrases, key) && typeof phrases[key] === "string") {
    return phrases[key];
  }
  let cur: unknown = tree;
  for (const part of key.split(".")) {
    if (cur == null || typeof cur !== "object" || !(part in cur)) {
      return undefined;
    }
    cur = (cur as Record<string, unknown>)[part];
  }
  return typeof cur === "string" ? cur : undefined;
}

export function interpolate(template: string, vars?: TranslateVars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (match, name: string) => {
    if (!Object.prototype.hasOwnProperty.call(vars, name)) return match;
    const value = vars[name];
    if (value == null || value === false) return "";
    if (typeof value === "string" || typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
      return String(value);
    }
    if (value instanceof Error) return value.message;
    return match;
  });
}

export function translate(locale: Locale, key: string, vars?: TranslateVars): string {
  const fromLocale = lookup(catalogs[locale], key);
  const fromDefault = locale === defaultLocale ? undefined : lookup(catalogs[defaultLocale], key);
  const fromEn = locale === "en" ? undefined : lookup(catalogs.en, key);
  const template = fromLocale ?? fromDefault ?? fromEn ?? key;
  return scrubBrand(interpolate(template, vars));
}

export function collectKeys(tree: unknown, prefix = ""): string[] {
  if (typeof tree === "string") {
    return prefix ? [prefix] : [];
  }
  if (tree == null || typeof tree !== "object") {
    return [];
  }
  const keys: string[] = [];
  for (const [k, v] of Object.entries(tree as Record<string, unknown>)) {
    if (k === "phrases" && !prefix && v != null && typeof v === "object") {
      for (const phrase of Object.keys(v as Record<string, unknown>)) {
        if (typeof (v as Record<string, unknown>)[phrase] === "string") keys.push(phrase);
      }
      continue;
    }
    const next = prefix ? `${prefix}.${k}` : k;
    keys.push(...collectKeys(v, next));
  }
  return keys;
}

export type TFunction = (key: string, vars?: TranslateVars) => string;

export const tDefault: TFunction = (key, vars) => translate(defaultLocale, key, vars);
