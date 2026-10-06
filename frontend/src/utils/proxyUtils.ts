import { getProxyUISettings } from "@/components/networking";
import { isQuietAccessDenial, readErrorMessage } from "@/lib/http/client";

export const fetchProxySettings = async (accessToken: string | null) => {
  if (!accessToken) return null;

  try {
    const proxySettings = await getProxyUISettings(accessToken);
    return proxySettings;
  } catch (error) {
    // A member is not supposed to read deployment settings. Missing them is
    // the normal case, not a page error.
    if (!isQuietAccessDenial(error)) {
      console.error("Error fetching proxy settings:", readErrorMessage(error) || String(error));
    }
    return null;
  }
};
