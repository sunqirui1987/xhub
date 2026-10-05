import { getUserBanner, UserBanner } from "@/components/networking";
import { useQuery, UseQueryOptions } from "@tanstack/react-query";
import { createQueryKeys } from "../common/queryKeysFactory";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { isProxyAdminRole } from "@/utils/roles";
import { t } from "@/i18n";

export const userBannerKeys = createQueryKeys("userBanner");

export const useUserBanner = (accessToken: string | null) => {
  const { userRole } = useAuthorized();
  const queryOptions: UseQueryOptions<UserBanner> = {
    queryKey: userBannerKeys.list({}),
    queryFn: async () => {
      if (!accessToken) {
        throw new Error(t("Access token is required"));
      }
      return await getUserBanner(accessToken);
    },
    enabled: Boolean(accessToken) && isProxyAdminRole(userRole),
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
  };
  return useQuery<UserBanner>(queryOptions);
};
