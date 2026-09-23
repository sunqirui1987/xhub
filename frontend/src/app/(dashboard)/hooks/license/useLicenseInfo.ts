import { useQuery, UseQueryResult } from "@tanstack/react-query";
import { LicenseInfo } from "@/components/networking";
import { createQueryKeys } from "../common/queryKeysFactory";

const licenseInfoKeys = createQueryKeys("licenseInfo");

export const useLicenseInfo = (accessToken: string | null | undefined): UseQueryResult<LicenseInfo | null> => {
  return useQuery<LicenseInfo | null>({
    queryKey: licenseInfoKeys.detail(accessToken ?? "none"),
    queryFn: async () => null,
    enabled: false,
  });
};
