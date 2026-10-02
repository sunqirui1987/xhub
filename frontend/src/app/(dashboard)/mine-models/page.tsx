"use client";

import { useQuery } from "@tanstack/react-query";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { userAvailableModelsCall } from "@/components/networking";
import type { MyModelCard } from "./grantedModelCards";
import { MyModels } from "./MyModels";

export default function MyModelsPage() {
  const { accessToken } = useAuthorized();
  const enabled = Boolean(accessToken);
  const available = useQuery({
    queryKey: ["mine-models", "available", accessToken],
    enabled,
    queryFn: () => userAvailableModelsCall(accessToken!),
  });
  const models = (available.data?.data ?? []) as MyModelCard[];
  return (
    <MyModels
      models={models}
      isLoading={available.isLoading}
      isError={available.isError}
      onRetry={() => {
        void available.refetch();
      }}
    />
  );
}
