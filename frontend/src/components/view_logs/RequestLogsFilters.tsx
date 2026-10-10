"use client";

import { useEffect, useMemo, useState } from "react";

import { useInfiniteSpendLogEndUsers } from "@/app/(dashboard)/hooks/spendLogs/useSpendLogEndUsers";
import { useInfiniteSpendLogUsers } from "@/app/(dashboard)/hooks/spendLogs/useSpendLogUsers";
import { useInfiniteKeyAliases } from "@/app/(dashboard)/hooks/keys/useKeyAliases";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { userAvailableModelsCall } from "@/components/networking";
import { DataTableFilterField } from "@/components/shared/DataTable";
import { PaginatedSearchSelect } from "@/components/shared/PaginatedSearchSelect";
import { SearchSelect, type SearchSelectOption } from "@/components/shared/SearchSelect";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

import useIsOrgAdmin from "@/app/(dashboard)/hooks/useIsOrgAdmin";
import { useIsPlatformAdmin, useIsTeamAdminForAnyTeam } from "@/app/(dashboard)/hooks/sessionIdentity/useSessionIdentity";
import type { Team } from "../key_team_helpers/key_list";
import { ERROR_CODE_OPTIONS } from "./constants";
import { LOG_FILTER_IDS, type LogsWindow } from "./log_filter_logic";
import { getActiveLocale, t } from "@/i18n";
import { translate } from "@/i18n/translate";

const ALL_VALUE = "all";

const STATUS_FILTER_KEYS = [
  { value: ALL_VALUE, label: "All Statuses" },
  { value: "success", label: "Success" },
] as const;

const CACHE_FILTER_KEYS = [
  { value: ALL_VALUE, label: "All Requests" },
  { value: "hit", label: "Cache Hit" },
  { value: "miss", label: "Cache Miss" },
] as const;
const PAGE_SIZE = 50;

const SEARCH_INPUT_REASONS: ReadonlySet<string> = new Set(["input-change", "input-clear", "clear-press"]);

const asString = (value: unknown): string => (typeof value === "string" ? value : "");
const emptyToUndefined = (value: string): string | undefined => (value === "" ? undefined : value);

function TeamFilterField({
  value,
  onChange,
  teams,
}: {
  value: string;
  onChange: (value: string | undefined) => void;
  teams: Team[];
}) {
  const options = useMemo<SearchSelectOption[]>(
    () =>
      teams.map((team) => ({
        label: team.team_alias || team.team_id,
        value: team.team_id,
        sublabel: team.team_id,
      })),
    [teams],
  );

  return (
    <DataTableFilterField label={t("Team ID")}>
      <SearchSelect
        options={options}
        value={value}
        onValueChange={(next) => onChange(next ?? undefined)}
        placeholder={t("Search or select a team")}
        emptyText={t("No teams found")}
      />
    </DataTableFilterField>
  );
}

function KeyAliasFilterField({
  value,
  onChange,
  teamId,
}: {
  value: string;
  onChange: (value: string | undefined) => void;
  teamId: string;
}) {
  const [search, setSearch] = useState("");
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteKeyAliases(
    PAGE_SIZE,
    emptyToUndefined(search),
    emptyToUndefined(teamId),
  );

  const options = useMemo<SearchSelectOption[]>(() => {
    const seen = new Set<string>();
    return (data?.pages ?? []).flatMap((page) =>
      page.aliases.flatMap((alias) => {
        if (!alias || seen.has(alias)) return [];
        seen.add(alias);
        return [{ label: alias, value: alias }];
      }),
    );
  }, [data]);

  return (
    <DataTableFilterField label={t("Key Alias")}>
      <PaginatedSearchSelect
        options={options}
        value={value}
        onValueChange={(next) => onChange(next ?? undefined)}
        onSearchChange={setSearch}
        onLoadMore={() => void fetchNextPage()}
        hasNextPage={hasNextPage}
        isLoading={isLoading}
        isFetchingNextPage={isFetchingNextPage}
        placeholder={t("Search a key alias")}
        emptyText={t("No key aliases found")}
      />
    </DataTableFilterField>
  );
}

/** 模型筛选：接收当前模型和查询回调，按登录凭据异步读取可用模型；退出登录立即隐藏选项，卸载时忽略迟到响应。 */
function ModelFilterField({ value, onChange }: { value: string; onChange: (value: string | undefined) => void }) {
  const { accessToken } = useAuthorized();
  const [options, setOptions] = useState<SearchSelectOption[]>([]);

  // Deployment ids come from /v2/model/info, which only a platform administrator
  // may call. The log row's model is the public model name, and /model/available
  // already returns that name to every signed-in account.
  useEffect(() => {
    if (!accessToken) return;
    let cancelled = false;
    userAvailableModelsCall(accessToken)
      .then((response) => {
        if (cancelled) return;
        const rows: Array<{ id?: string | null }> = Array.isArray(response?.data) ? response.data : [];
        const seen = new Set<string>();
        setOptions(
          rows.flatMap((model) => {
            const id = model?.id ?? "";
            if (!id || seen.has(id)) return [];
            seen.add(id);
            return [{ label: id, value: id }];
          }),
        );
      })
      .catch(() => {
        if (!cancelled) setOptions([]);
      });
    return () => {
      cancelled = true;
    };
  }, [accessToken]);

  return (
    <DataTableFilterField label={t("Model")}>
      <SearchSelect
        options={accessToken ? options : []}
        value={value}
        onValueChange={(next) => onChange(next ?? undefined)}
        placeholder={t("Search a model")}
        emptyText={t("No models found")}
      />
    </DataTableFilterField>
  );
}

function UserIdFilterField({
  value,
  onChange,
  logsWindow,
}: {
  value: string;
  onChange: (value: string | undefined) => void;
  logsWindow: LogsWindow;
}) {
  const [search, setSearch] = useState("");
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteSpendLogUsers(
    logsWindow,
    PAGE_SIZE,
    emptyToUndefined(search),
  );

  const options = useMemo<SearchSelectOption[]>(() => {
    const seen = new Set<string>();
    return (data?.pages ?? []).flatMap((page) =>
      page.data.flatMap((userId) => {
        if (!userId || seen.has(userId)) return [];
        seen.add(userId);
        return [{ label: userId, value: userId }];
      }),
    );
  }, [data]);

  return (
    <DataTableFilterField label={t("User ID")}>
      <PaginatedSearchSelect
        options={options}
        value={value}
        onValueChange={(next) => onChange(next ?? undefined)}
        onSearchChange={setSearch}
        onLoadMore={() => void fetchNextPage()}
        hasNextPage={hasNextPage}
        isLoading={isLoading}
        isFetchingNextPage={isFetchingNextPage}
        placeholder={t("Search an internal user")}
        emptyText={t("No users found")}
      />
    </DataTableFilterField>
  );
}

function EndUserFilterField({
  value,
  onChange,
  logsWindow,
}: {
  value: string;
  onChange: (value: string | undefined) => void;
  logsWindow: LogsWindow;
}) {
  const [search, setSearch] = useState("");
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteSpendLogEndUsers(
    logsWindow,
    PAGE_SIZE,
    emptyToUndefined(search),
  );

  const options = useMemo<SearchSelectOption[]>(() => {
    const seen = new Set<string>();
    return (data?.pages ?? []).flatMap((page) =>
      page.data.flatMap((endUser) => {
        if (!endUser || seen.has(endUser)) return [];
        seen.add(endUser);
        return [{ label: endUser, value: endUser }];
      }),
    );
  }, [data]);

  return (
    <DataTableFilterField label={t("End User")}>
      <PaginatedSearchSelect
        options={options}
        value={value}
        onValueChange={(next) => onChange(next ?? undefined)}
        onSearchChange={setSearch}
        onLoadMore={() => void fetchNextPage()}
        hasNextPage={hasNextPage}
        isLoading={isLoading}
        isFetchingNextPage={isFetchingNextPage}
        placeholder={t("Search an end user")}
        emptyText={t("No end users in this time range")}
      />
    </DataTableFilterField>
  );
}

/** 错误码筛选：接收当前 HTTP 码及变更回调，按页面语言显示已知码且支持自定义码；仅变更查询，不写入后台。 */
function ErrorCodeFilterField({ value, onChange }: { value: string; onChange: (value: string | undefined) => void }) {
  const [query, setQuery] = useState("");
  const locale = getActiveLocale();
  const errorCodeOptions = useMemo(() => ERROR_CODE_OPTIONS.map((option) => ({ ...option, label: translate(locale, option.label) })), [locale]);

  const options = useMemo<SearchSelectOption[]>(() => {
    const trimmed = query.trim();
    const lowered = trimmed.toLowerCase();
    const matches = errorCodeOptions.filter((option) => option.label.toLowerCase().includes(lowered));
    const isKnownCode = errorCodeOptions.some(
      (option) => option.value === trimmed || option.label.toLowerCase() === lowered,
    );
    if (trimmed === "" || isKnownCode) return matches;
    return [...matches, { label: t("Use custom code: {trimmed}", { trimmed }), value: trimmed }];
  }, [query, errorCodeOptions]);

  const selected = useMemo<SearchSelectOption | null>(() => {
    if (value === "") return null;
    return errorCodeOptions.find((option) => option.value === value) ?? { label: value, value };
  }, [value, errorCodeOptions]);

  const items = useMemo<SearchSelectOption[]>(() => {
    if (selected === null) return options;
    if (options.some((option) => option.value === selected.value)) return options;
    return [selected, ...options];
  }, [options, selected]);

  return (
    <DataTableFilterField label={t("Error Code")}>
      <Combobox
        items={items}
        value={selected}
        onValueChange={(item: SearchSelectOption | null) => onChange(emptyToUndefined(item?.value ?? ""))}
        onInputValueChange={(next, eventDetails) => setQuery(SEARCH_INPUT_REASONS.has(eventDetails.reason) ? next : "")}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setQuery("");
        }}
        isItemEqualToValue={(a: SearchSelectOption, b: SearchSelectOption) => a.value === b.value}
        itemToStringLabel={(item: SearchSelectOption) => item.label}
        filter={null}
      >
        <ComboboxInput
          onFocus={(event) => event.currentTarget.select()}
          placeholder={t("Select or type an error code")}
          showClear={value !== ""}
          className="w-full"
        />
        <ComboboxContent>
          <ComboboxEmpty>{t("No error codes found")}</ComboboxEmpty>
          <ComboboxList data-testid="error-code-filter-list">
            {(item: SearchSelectOption) => (
              <ComboboxItem key={item.value} value={item}>
                {item.label}
              </ComboboxItem>
            )}
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </DataTableFilterField>
  );
}

interface RequestLogsFiltersProps {
  get: (columnId: string) => unknown;
  set: (columnId: string, value: unknown) => void;
  teams: Team[];
  logsWindow: LogsWindow;
  errorsOnly?: boolean;
}

/** 渲染日志筛选项；接收表格读写回调和授权范围，普通视图不提供失败选项，错误视图隐藏状态选择；返回筛选表单，无数据写入副作用。 */
export function RequestLogsFilters({ get, set, teams, logsWindow, errorsOnly = false }: RequestLogsFiltersProps) {
  // 标签随当前页面语言翻译，不能在模块加载时固定为默认语言。
  const STATUS_FILTER_ITEMS = STATUS_FILTER_KEYS.map((item) => ({ ...item, label: t(item.label) }));
  const CACHE_FILTER_ITEMS = CACHE_FILTER_KEYS.map((item) => ({ ...item, label: t(item.label) }));
  const valueOf = (id: string): string => asString(get(id));
  const setter = (id: string) => (next: string | undefined) => set(id, next);
  // 三种权限 Hook 必须每次都执行，避免角色变化导致 Hook 调用顺序改变。
  const platformAdmin = useIsPlatformAdmin();
  const orgAdmin = useIsOrgAdmin();
  const teamAdmin = useIsTeamAdminForAnyTeam();
  const canChooseScope = platformAdmin || orgAdmin || teamAdmin;

  return (
    <>
      {canChooseScope ? (
        <TeamFilterField
          value={valueOf(LOG_FILTER_IDS.TEAM_ID)}
          onChange={setter(LOG_FILTER_IDS.TEAM_ID)}
          teams={teams}
        />
      ) : null}

      {!errorsOnly && <DataTableFilterField label={t("Status")}>
        <Select
          items={STATUS_FILTER_ITEMS}
          // 兼容旧页面保存的失败筛选，普通日志只允许成功或全部非失败状态。
          value={valueOf(LOG_FILTER_IDS.STATUS) === "success" ? "success" : ALL_VALUE}
          onValueChange={(next) => set(LOG_FILTER_IDS.STATUS, next === null || next === ALL_VALUE ? undefined : next)}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={t("All Statuses")} />
          </SelectTrigger>
          <SelectContent>
            {STATUS_FILTER_ITEMS.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </DataTableFilterField>}

      <DataTableFilterField label={t("Cache")}>
        <Select
          items={CACHE_FILTER_ITEMS}
          value={valueOf(LOG_FILTER_IDS.CACHE_STATUS) === "" ? ALL_VALUE : valueOf(LOG_FILTER_IDS.CACHE_STATUS)}
          onValueChange={(next) =>
            set(LOG_FILTER_IDS.CACHE_STATUS, next === null || next === ALL_VALUE ? undefined : next)
          }
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder={t("All Requests")} />
          </SelectTrigger>
          <SelectContent>
            {CACHE_FILTER_ITEMS.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </DataTableFilterField>

      <KeyAliasFilterField
        value={valueOf(LOG_FILTER_IDS.KEY_ALIAS)}
        onChange={setter(LOG_FILTER_IDS.KEY_ALIAS)}
        teamId={valueOf(LOG_FILTER_IDS.TEAM_ID)}
      />

      {canChooseScope ? (
        <UserIdFilterField
          value={valueOf(LOG_FILTER_IDS.USER_ID)}
          onChange={setter(LOG_FILTER_IDS.USER_ID)}
          logsWindow={logsWindow}
        />
      ) : null}

      <EndUserFilterField
        value={valueOf(LOG_FILTER_IDS.END_USER)}
        onChange={setter(LOG_FILTER_IDS.END_USER)}
        logsWindow={logsWindow}
      />

      <ErrorCodeFilterField value={valueOf(LOG_FILTER_IDS.ERROR_CODE)} onChange={setter(LOG_FILTER_IDS.ERROR_CODE)} />

      <DataTableFilterField label={t("Error Message")}>
        <Input
          value={valueOf(LOG_FILTER_IDS.ERROR_MESSAGE)}
          onChange={(event) => set(LOG_FILTER_IDS.ERROR_MESSAGE, emptyToUndefined(event.target.value))}
          placeholder={t("Enter error message…")}
        />
      </DataTableFilterField>

      <DataTableFilterField label={t("Key Hash")}>
        <Input
          value={valueOf(LOG_FILTER_IDS.KEY_HASH)}
          onChange={(event) => set(LOG_FILTER_IDS.KEY_HASH, emptyToUndefined(event.target.value))}
          placeholder={t("Enter key hash…")}
        />
      </DataTableFilterField>

      <DataTableFilterField label={t("Session ID")}>
        <Input
          value={valueOf(LOG_FILTER_IDS.SESSION_ID)}
          onChange={(event) => set(LOG_FILTER_IDS.SESSION_ID, emptyToUndefined(event.target.value))}
          placeholder={t("Enter session ID…")}
        />
      </DataTableFilterField>

      <ModelFilterField value={valueOf(LOG_FILTER_IDS.MODEL_ID)} onChange={setter(LOG_FILTER_IDS.MODEL_ID)} />

      <DataTableFilterField label={t("Public model / search tool")}>
        <Input
          value={valueOf(LOG_FILTER_IDS.PUBLIC_MODEL_OR_SEARCH_TOOL)}
          onChange={(event) => set(LOG_FILTER_IDS.PUBLIC_MODEL_OR_SEARCH_TOOL, emptyToUndefined(event.target.value))}
          placeholder={t("Enter public model or search tool…")}
        />
      </DataTableFilterField>
    </>
  );
}
