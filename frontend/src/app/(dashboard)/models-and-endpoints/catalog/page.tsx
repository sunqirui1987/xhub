"use client";

import { useSearchParams } from "next/navigation";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { ModelCatalog } from "./ModelCatalog";

export default function ModelCatalogPage() {
  const params = useSearchParams();
  const provider = params.get("provider") === "qiniu" ? "qiniu" : "fennoai";
  const { accessToken } = useAuthorized();
  if (!accessToken) return null;
  return <ModelCatalog provider={provider} accessToken={accessToken} />;
}
