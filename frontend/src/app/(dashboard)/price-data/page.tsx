"use client";

import LoadingScreen from "@/components/common_components/LoadingScreen";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { isAdminRole } from "@/utils/roles";
import { uiHref } from "@/utils/uiHref";
import PriceDataManagementTab from "@/app/(dashboard)/models-and-endpoints/components/PriceDataManagementTab";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function PriceDataPage() {
  const router = useRouter();
  const { userRole, isViewOnly, isLoading } = useAuthorized();
  const canViewPriceData = isAdminRole(userRole) && !isViewOnly;

  useEffect(() => {
    if (!isLoading && !canViewPriceData) {
      router.replace(uiHref("api-keys"));
    }
  }, [canViewPriceData, isLoading, router]);

  if (isLoading || !canViewPriceData) {
    return <LoadingScreen />;
  }

  return <PriceDataManagementTab />;
}
