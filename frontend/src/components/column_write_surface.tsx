"use client";

import { useCallback, useEffect, useState } from "react";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { apiClient } from "@/components/networking";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export interface ColumnAction {
  label: string;
  method: "POST" | "PUT" | "PATCH" | "DELETE";
  path: string | ((ctx: { name: string; id: string }) => string);
  body?: (ctx: { name: string; id: string }) => unknown;
  needsId?: boolean;
}

const ID_KEYS = [
  "id",
  "agent_id",
  "server_id",
  "skill_id",
  "policy_id",
  "run_id",
  "budget_id",
  "prompt_id",
  "vector_store_id",
  "search_tool_id",
  "key",
];

const readId = (data: unknown): string => {
  if (data == null || typeof data !== "object") return "";
  const record = data as Record<string, unknown>;
  for (const key of ID_KEYS) {
    if (typeof record[key] === "string" && record[key] !== "") return record[key] as string;
  }
  return "";
};

export function ColumnWriteSurface({
  title,
  emptyText,
  listPath,
  actions,
}: {
  title: string;
  emptyText: string;
  listPath: string;
  actions: ColumnAction[];
}) {
  const { accessToken } = useAuthorized();
  const [name, setName] = useState("");
  const [id, setId] = useState("");
  const [listText, setListText] = useState("");
  const [result, setResult] = useState("");

  const load = useCallback(async () => {
    if (!accessToken) return;
    try {
      const data = await apiClient.get(listPath, { accessToken });
      setListText(typeof data === "string" ? data : JSON.stringify(data));
    } catch (error) {
      setListText(error instanceof Error ? error.message : String(error));
    }
  }, [accessToken, listPath]);

  useEffect(() => {
    void load();
  }, [load]);

  const run = async (action: ColumnAction) => {
    setResult("");
    if (!accessToken) {
      setResult("缺少登录会话");
      return;
    }
    if (action.needsId && id === "") {
      setResult("请先创建，再更新或删除");
      return;
    }
    const ctx = { name, id };
    const path = typeof action.path === "function" ? action.path(ctx) : action.path;
    try {
      const data = await apiClient.request(action.method, path, {
        accessToken,
        body: action.body ? action.body(ctx) : undefined,
      });
      const text = typeof data === "string" ? data : JSON.stringify(data);
      setResult(text);
      const nextId = readId(data);
      if (nextId) setId(nextId);
    } catch (error) {
      setResult(error instanceof Error ? error.message : String(error));
    }
    await load();
  };

  const empty = listText === "" || listText === "[]" || listText === "null";

  return (
    <div className="space-y-4 p-6">
      <h1 className="text-2xl font-semibold">{title}</h1>
      {empty ? <p>{emptyText}</p> : null}
      <pre data-testid="column-list" className="max-h-48 overflow-auto rounded bg-muted p-3 text-xs">
        {listText}
      </pre>
      <Input aria-label="名称" value={name} onChange={(event) => setName(event.target.value)} placeholder="名称" />
      <div className="flex gap-2">
        {actions.map((action) => (
          <Button key={action.label} type="button" onClick={() => void run(action)}>
            {action.label}
          </Button>
        ))}
      </div>
      <p data-testid="column-result">{result}</p>
    </div>
  );
}
