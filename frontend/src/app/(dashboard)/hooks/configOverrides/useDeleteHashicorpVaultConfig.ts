import { deleteHashicorpVaultConfig } from "./hashicorpVaultApi";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { hashicorpVaultKeys } from "./useHashicorpVaultConfig";
import { t } from "@/i18n";

export const useDeleteHashicorpVaultConfig = (accessToken: string | null) => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      if (!accessToken) {
        throw new Error(t("Access token is required"));
      }
      return deleteHashicorpVaultConfig(accessToken);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: hashicorpVaultKeys.all });
    },
  });
};
