import { updateHashicorpVaultConfig } from "./hashicorpVaultApi";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { hashicorpVaultKeys } from "./useHashicorpVaultConfig";
import { t } from "@/i18n";

export const useUpdateHashicorpVaultConfig = (accessToken: string | null) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (config: Record<string, any>) => {
      if (!accessToken) {
        throw new Error(t("Access token is required"));
      }
      return updateHashicorpVaultConfig(accessToken, config);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: hashicorpVaultKeys.all });
    },
  });
};
