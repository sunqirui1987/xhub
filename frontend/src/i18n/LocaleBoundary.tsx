"use client";

import React, { useSyncExternalStore } from "react";
import { getActiveLocale, subscribeLocale } from "./runtime";

// Components call the module-level t() and do not subscribe to locale. A memoized
// parent then keeps their last Chinese render. Remounting on locale change makes
// every one of them read the new language.
export function LocaleBoundary({ children }: { children: React.ReactNode }) {
  const locale = useSyncExternalStore(subscribeLocale, getActiveLocale, getActiveLocale);
  return <React.Fragment key={locale}>{children}</React.Fragment>;
}
