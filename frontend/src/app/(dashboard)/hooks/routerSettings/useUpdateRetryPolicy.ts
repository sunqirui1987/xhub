import { setCallbacksCall } from "@/components/networking";
import { useMutation } from "@tanstack/react-query";
import { t } from "@/i18n";

export interface RetryPolicyPayload {
  retry_policy?: Record<string, number> | null;
  model_group_retry_policy?: Record<string, Record<string, number> | undefined> | null;
}

export const useUpdateRetryPolicy = (accessToken: string | null) =>
  useMutation({
    mutationFn: async (policy: RetryPolicyPayload) => {
      if (!accessToken) {
        throw new Error(t("Access token is required"));
      }
      return setCallbacksCall(accessToken, { router_settings: policy });
    },
  });
