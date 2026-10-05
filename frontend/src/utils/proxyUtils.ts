import { getProxyUISettings } from "@/components/networking";

const managementDenied = (error: unknown): boolean => {
  const message = error instanceof Error ? error.message : String(error ?? "");
  return message.includes("Not allowed to access management endpoints");
};

export const fetchProxySettings = async (accessToken: string | null) => {
  if (!accessToken) return null;

  try {
    const proxySettings = await getProxyUISettings(accessToken);
    return proxySettings;
  } catch (error) {
    // A member is not supposed to read deployment settings. Missing them is
    // the normal case, not a page error.
    if (!managementDenied(error)) {
      console.error("Error fetching proxy settings:", error instanceof Error ? error.message : error);
    }
    return null;
  }
};
