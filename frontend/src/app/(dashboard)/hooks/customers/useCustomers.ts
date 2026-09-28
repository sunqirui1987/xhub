import { $api } from "@/lib/http/api";
import { all_admin_roles } from "@/utils/roles";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import type { components } from "@/lib/http/schema";

export type EndUser = components["schemas"]["CustomerResponse"];

// The list endpoint is documented as an array and also returns { customers, end_users, data }.
export function customerListFrom(data: unknown): EndUser[] {
  if (Array.isArray(data)) return data as EndUser[];
  if (data && typeof data === "object") {
    const doc = data as Record<string, unknown>;
    for (const key of ["customers", "end_users", "data"]) {
      if (Array.isArray(doc[key])) return doc[key] as EndUser[];
    }
  }
  return [];
}

export const useCustomers = () => {
  const { accessToken, userRole } = useAuthorized();
  return $api.useQuery(
    "get",
    "/customer/list",
    {},
    {
      enabled: Boolean(accessToken) && all_admin_roles.includes(userRole!),
      select: (data) => customerListFrom(data),
    },
  );
};
