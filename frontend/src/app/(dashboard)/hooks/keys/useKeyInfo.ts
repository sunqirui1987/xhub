import { useQuery, UseQueryResult } from "@tanstack/react-query";

import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { KeyResponse } from "@/components/key_team_helpers/key_list";
import { getProxyBaseUrl, getGlobalLitellmHeaderName, keyInfoV1Call } from "@/components/networking";
import { createApiClient } from "@/lib/http/client";

import { keyKeys } from "./useKeys";
import { t } from "@/i18n";

// 越界详情 404 属于正常拒绝，不触发全局错误弹窗；共享客户端保留状态码及认证头。
const personalClient = createApiClient({ getBaseUrl: getProxyBaseUrl, getAuthHeaderName: getGlobalLitellmHeaderName });

/** 读取密钥详情；参数为密钥 ID 和启用/个人范围选项，返回查询状态；个人详情由后台验证本人归属，404 等错误抛出。供密钥详情页面调用，缓存按会话与范围隔离。 */
export function useKeyInfo(keyId: string | null, options?: { enabled?: boolean; scope?: "personal" }): UseQueryResult<KeyResponse> {
  const { accessToken } = useAuthorized();

  return useQuery<KeyResponse>({
    queryKey: [...keyKeys.detail(keyId ?? ""), accessToken, options?.scope],
    queryFn: async () => {
      if (!accessToken || !keyId) throw new Error(t("Missing access token or key id"));
      let keyData;
      if (options?.scope === "personal") {
        keyData = await personalClient.get("/key/info", {
          accessToken,
          query: { key: keyId, scope: "personal" },
        });
      } else {
        keyData = await keyInfoV1Call(accessToken, keyId);
      }
      return {
        ...keyData["info"],
        token: keyId,
        api_key: keyId,
      };
    },
    enabled: Boolean(accessToken && keyId) && (options?.enabled ?? true),
  });
}
