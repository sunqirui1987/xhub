import { describe, expect, it, vi } from "vitest";

import { useMCPServers } from "./useMCPServers";

vi.mock("@/components/networking", () => ({
  fetchMCPServers: vi.fn(),
}));

describe("useMCPServers", () => {
  it("does not call the MCP API", () => {
    const result = useMCPServers();
    expect(result.data).toEqual([]);
    expect(result.isLoading).toBe(false);
  });
});
