import { getPriceCatalog } from "@/components/networking";
import { useQuery } from "@tanstack/react-query";
import { createQueryKeys } from "../common/queryKeysFactory";
import type { PriceCatalogDocument } from "../../models-and-endpoints/components/priceCatalogRows";

export const priceCatalogKeys = createQueryKeys("priceCatalog");

/**
 * The price catalog for the console. Unlike the public cost map this is
 * management-gated, because the same response says which rows an operator has
 * overridden, and the page needs that to offer a reset.
 */
export const usePriceCatalog = (accessToken: string | null) => {
  return useQuery<PriceCatalogDocument>({
    queryKey: priceCatalogKeys.list({}),
    queryFn: async () => (await getPriceCatalog(accessToken ?? "")) as PriceCatalogDocument,
    enabled: Boolean(accessToken),
    staleTime: 30 * 1000,
    gcTime: 5 * 60 * 1000,
  });
};
