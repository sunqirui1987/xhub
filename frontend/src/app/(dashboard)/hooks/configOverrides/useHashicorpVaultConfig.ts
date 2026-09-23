import { getHashicorpVaultConfig } from "./hashicorpVaultApi";
import { useQuery } from "@tanstack/react-query";
import useAuthorized from "../useAuthorized";
import { createQueryKeys } from "../common/queryKeysFactory";
import { t } from "@/i18n";

export const hashicorpVaultKeys = createQueryKeys("hashicorpVaultConfig");

export const useHashicorpVaultConfig = () => {
  const { accessToken } = useAuthorized();

  return useQuery<Record<string, any>>({
    queryKey: hashicorpVaultKeys.list({}),
    queryFn: async () => {
      if (!accessToken) {
        throw new Error(t("Access token is required"));
      }
      return getHashicorpVaultConfig(accessToken);
    },
    enabled: !!accessToken,
    staleTime: 60 * 60 * 1000,
    gcTime: 60 * 60 * 1000,
  });
};
