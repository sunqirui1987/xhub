"use client";

import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import moment from "moment";
import "moment/locale/zh-cn";
import { defaultLocale, LOCALE_COOKIE, parseLocale, translate, type Locale, type TFunction, type TranslateVars } from "./translate";
import { getActiveLocale, setActiveLocale, t as runtimeT } from "./runtime";

function applyMomentLocale(locale: Locale) {
  moment.locale(locale === "zh-CN" ? "zh-cn" : "en");
}

type I18nContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: TFunction;
};

const I18nContext = createContext<I18nContextValue | null>(null);

function persistLocale(locale: Locale) {
  if (typeof document !== "undefined") {
    document.cookie = `${LOCALE_COOKIE}=${locale}; path=/; max-age=31536000; SameSite=Lax`;
    document.documentElement.lang = locale;
  }
  try {
    window.localStorage.setItem(LOCALE_COOKIE, locale);
  } catch {
    /* ignore quota / private mode */
  }
}

function readStoredLocale(): Locale | null {
  if (typeof window === "undefined") return null;
  try {
    const stored = window.localStorage.getItem(LOCALE_COOKIE);
    if (stored) return parseLocale(stored);
  } catch {
    /* ignore */
  }
  const match = document.cookie.match(new RegExp(`(?:^|; )${LOCALE_COOKIE}=([^;]*)`));
  return match ? parseLocale(decodeURIComponent(match[1])) : null;
}

export function I18nProvider({
  children,
  initialLocale,
}: {
  children: React.ReactNode;
  initialLocale?: Locale;
}) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    const next = initialLocale ?? readStoredLocale() ?? getActiveLocale() ?? defaultLocale;
    setActiveLocale(next);
    applyMomentLocale(next);
    return next;
  });

  useEffect(() => {
    const stored = readStoredLocale();
    if (stored && stored !== locale) {
      setLocaleState(stored);
    }
    document.documentElement.lang = stored ?? locale;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- hydrate from storage once
  }, []);

  const setLocale = useCallback((next: Locale) => {
    setActiveLocale(next);
    applyMomentLocale(next);
    persistLocale(next);
    setLocaleState(next);
  }, []);

  useEffect(() => {
    setActiveLocale(locale);
    applyMomentLocale(locale);
  }, [locale]);

  const t = useCallback<TFunction>((key: string, vars?: TranslateVars) => translate(locale, key, vars), [locale]);

  const value = useMemo(() => ({ locale, setLocale, t }), [locale, setLocale, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (ctx) return ctx;
  return {
    locale: defaultLocale,
    setLocale: () => {},
    t: runtimeT,
  };
}

export function useT(): TFunction {
  return useI18n().t;
}
