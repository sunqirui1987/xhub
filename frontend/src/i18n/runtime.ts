import { defaultLocale, translate, type Locale, type TranslateVars } from "./translate";

let activeLocale: Locale = defaultLocale;
const listeners = new Set<() => void>();

export function getActiveLocale(): Locale {
  return activeLocale;
}

export function setActiveLocale(locale: Locale): void {
  if (activeLocale === locale) return;
  activeLocale = locale;
  listeners.forEach((fn) => fn());
}

export function subscribeLocale(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function t(key: string, vars?: TranslateVars): string {
  return translate(activeLocale, key, vars);
}
