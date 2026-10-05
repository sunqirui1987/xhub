"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { uiHref } from "@/utils/uiHref";

export default function IntegrationsPage() {
  const router = useRouter();
  useEffect(() => {
    router.replace(uiHref("chat"));
  }, [router]);
  return null;
}
