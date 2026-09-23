import { updateCyberArkConfig } from "./cyberArkApi";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { cyberArkKeys } from "./useCyberArkConfig";
import { t } from "@/i18n";

export const useUpdateCyberArkConfig = (accessToken: string | null) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (config: Record<string, string>) => {
      if (!accessToken) {
        throw new Error(t("Access token is required"));
      }
      return updateCyberArkConfig(accessToken, config);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cyberArkKeys.all });
    },
  });
};
