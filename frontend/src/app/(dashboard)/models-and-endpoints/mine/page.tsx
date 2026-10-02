"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { uiHref } from "@/utils/uiHref";

export default function LegacyMyModelsRedirect() {
  const router = useRouter();
  useEffect(() => {
    router.replace(uiHref("mine-models"));
  }, [router]);
  return null;
}
