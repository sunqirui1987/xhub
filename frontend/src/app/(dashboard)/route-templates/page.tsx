"use client";

import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import RouteTemplatesPanel from "./_components/RouteTemplatesPanel";

export default function RouteTemplatesPage() {
  const { accessToken } = useAuthorized();
  return <RouteTemplatesPanel accessToken={accessToken} />;
}
