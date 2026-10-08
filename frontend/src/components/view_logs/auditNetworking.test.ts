import { afterEach, describe, expect, it, vi } from "vitest";
import { uiAuditLogsCall } from "../networking";

afterEach(() => vi.unstubAllGlobals());

describe("gateway audit logs", () => {
  it("requests the mounted route and maps gateway records without inventing a diff", async () => {
    const detail = { status: "inactive" };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        audit_logs: [
          {
            id: 7,
            ts: "2026-10-08T01:00:00Z",
            actor_id: "admin-1",
            actor_kind: "session",
            actor_name: "Alice",
            actor_email: "alice@example.com",
            object_name: "Production key",
            team_id: "team-1",
            action: "key.status",
            object_type: "key",
            object_id: "key-1",
            detail,
          },
          {
            id: 6,
            ts: "2026-10-08T00:00:00Z",
            actor_id: "key-2",
            actor_kind: "key",
            action: "log.read",
            object_type: "request_log",
            object_id: "request-1",
            detail: null,
          },
        ],
        total: 3,
        page: 2,
        page_size: 2,
        total_pages: 2,
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    const data = await uiAuditLogsCall({
      accessToken: "sk-test",
      page: 2,
      page_size: 2,
      params: { changed_by: "admin-1", search: "key-1", object_id: undefined },
    });
    const url = new URL(fetchMock.mock.calls[0][0], "http://localhost");
    expect(url.pathname).toBe("/audit/logs");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      page: "2",
      page_size: "2",
      changed_by: "admin-1",
      search: "key-1",
    });
    expect(data).toMatchObject({ total: 3, page: 2, page_size: 2, total_pages: 2 });
    expect(data.audit_logs[0]).toEqual({
      id: "7",
      updated_at: "2026-10-08T01:00:00Z",
      changed_by: "admin-1",
      actor_kind: "session",
      actor_name: "Alice",
      actor_email: "alice@example.com",
      object_name: "Production key",
      team_id: "team-1",
      changed_by_api_key: "",
      action: "key.status",
      table_name: "key",
      object_id: "key-1",
      before_value: {},
      updated_values: {},
      detail,
    });
    expect(data.audit_logs[1]).toMatchObject({ id: "6", changed_by_api_key: "key-2", detail: {} });
  });
});
