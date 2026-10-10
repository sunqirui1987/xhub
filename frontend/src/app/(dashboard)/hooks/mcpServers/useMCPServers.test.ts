import { describe, expect, it, vi } from "vitest";

import { useMCPServers } from "./useMCPServers";
import { fetchMCPServers } from "@/components/networking";

vi.mock("@/components/networking", () => ({
  fetchMCPServers: vi.fn(),
}));

describe("useMCPServers", () => {
  /** 无团队及历史团队 ID 都返回稳定空目录，不请求退役接口；每次清理 mock 调用。 */
  it.each([undefined, null, "legacy-team"])("keeps MCP unsupported for team %s", async (teamId) => {
    vi.clearAllMocks();
    const result = useMCPServers(teamId);
    expect(result.data).toEqual([]);
    expect(result.isLoading).toBe(false);
    expect(result.isError).toBe(false);
    expect(result.error).toBeNull();
    expect(await result.refetch()).toEqual({ data: [] });
    expect(fetchMCPServers).not.toHaveBeenCalled();
  });
});
