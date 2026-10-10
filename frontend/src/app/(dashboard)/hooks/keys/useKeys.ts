import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
  UseQueryResult,
  type InfiniteData,
  type QueryKey,
} from "@tanstack/react-query";
import { createQueryKeys } from "../common/queryKeysFactory";
import { getProxyBaseUrl, getGlobalLitellmHeaderName, deriveErrorMessage, handleError } from "@/components/networking";
import { KeyResponse } from "@/components/key_team_helpers/key_list";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { t } from "@/i18n";

export const keyKeys = createQueryKeys("keys");

export interface KeysResponse {
  keys: KeyResponse[];
  total_count: number;
  current_page: number;
  total_pages: number;
}

export interface DeletedKeyResponse extends KeyResponse {
  deleted_at: string;
  deleted_by: string;
}

export interface DeletedKeysResponse {
  keys: DeletedKeyResponse[];
  total_count: number;
  current_page: number;
  total_pages: number;
}

export interface KeyListCallOptions {
  scope?: "personal";
  organizationID?: string | null;
  teamID?: string | null;
  projectID?: string | null;
  agentID?: string | null;
  selectedKeyAlias?: string | null;
  userID?: string | null;
  keyHash?: string | null;
  search?: string | null;
  sortBy?: string | null;
  sortOrder?: string | null;
  expand?: string | null;
  status?: string | null;
}

/** 请求密钥列表；参数为会话、页码、页大小和筛选项，返回分页结果；个人范围由后台按会话强制隔离，网络或接口失败抛错。供密钥查询 hooks 调用。 */
const keyListCall = async (accessToken: string, page: number, pageSize: number, options: KeyListCallOptions = {}) => {
  try {
    const baseUrl = getProxyBaseUrl();

    const params = new URLSearchParams(
      Object.entries({
        scope: options.scope,
        team_id: options.teamID,
        project_id: options.projectID,
        agent_id: options.agentID,
        organization_id: options.organizationID,
        key_alias: options.selectedKeyAlias,
        key_hash: options.keyHash,
        search: options.search,
        user_id: options.userID,
        page,
        size: pageSize,
        sort_by: options.sortBy,
        sort_order: options.sortOrder,
        expand: options.expand,
        status: options.status,
        return_full_object: "true",
        include_team_keys: "true",
        include_created_by_keys: "true",
        // Opt into substring matching so the admin key-list search box keeps
        // matching partial user_id/key_alias. /key/list is exact by default.
        substring_matching: "true",
      })
        .filter(([, value]) => value !== undefined && value !== null)
        .map(([key, value]) => [key, String(value)]),
    );

    const url = `${baseUrl ? `${baseUrl}/key/list` : "/key/list"}?${params}`;

    const response = await fetch(url, {
      method: "GET",
      headers: {
        [getGlobalLitellmHeaderName()]: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
    });

    if (!response.ok) {
      const errorData = await response.json();
      const errorMessage = deriveErrorMessage(errorData);
      handleError(errorMessage);
      throw new Error(errorMessage);
    }

    const data = await response.json();
    return data;
  } catch (error) {
    console.error("Failed to list keys:", error);
    throw error;
  }
};

/** 查询指定范围的密钥；参数为分页与筛选，返回查询状态；按会话分开缓存，个人查询缺少用户身份时不发送。供密钥页面调用。 */
export const useKeys = (
  page: number,
  pageSize: number,
  options: KeyListCallOptions = {},
): UseQueryResult<KeysResponse> => {
  const { accessToken, userId } = useAuthorized();

  return useQuery<KeysResponse>({
    queryKey: [...keyKeys.list({ page, limit: pageSize, ...options }), accessToken],
    queryFn: async () => await keyListCall(accessToken!, page, pageSize, options),
    enabled: Boolean(accessToken) && (options.scope !== "personal" || Boolean(userId)),
    staleTime: 30000, // 30 seconds
    placeholderData: options.scope === "personal" ? undefined : keepPreviousData,
  });
};

const infiniteKeyKeys = createQueryKeys("infiniteKeys");

export const useInfiniteKeys = (pageSize: number, options: KeyListCallOptions = {}) => {
  const { accessToken } = useAuthorized();

  const infiniteKeyListOptions = {
    queryKey: infiniteKeyKeys.list({ limit: pageSize, ...options }),
    queryFn: async ({ pageParam }: { pageParam: number }) => {
      if (!accessToken) throw new Error(t("Access token required"));
      return await keyListCall(accessToken, pageParam, pageSize, options);
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage: KeysResponse) =>
      lastPage.current_page < lastPage.total_pages ? lastPage.current_page + 1 : undefined,
    enabled: Boolean(accessToken),
    staleTime: 30_000,
  };

  return useInfiniteQuery<KeysResponse, Error, InfiniteData<KeysResponse>, QueryKey, number>(infiniteKeyListOptions);
};

export const deletedKeyKeys = createQueryKeys("deletedKeys");
export const useDeletedKeys = (
  page: number,
  pageSize: number,
  options: KeyListCallOptions = {},
): UseQueryResult<DeletedKeysResponse> => {
  const { accessToken } = useAuthorized();

  return useQuery<DeletedKeysResponse>({
    queryKey: deletedKeyKeys.list({ page, limit: pageSize, ...options }),
    queryFn: async () => await keyListCall(accessToken!, page, pageSize, { ...options, status: "deleted" }),
    enabled: Boolean(accessToken),
    staleTime: 30000, // 30 seconds
    placeholderData: keepPreviousData,
  });
};
