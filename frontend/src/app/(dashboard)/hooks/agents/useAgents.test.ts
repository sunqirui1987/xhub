import { describe, expect, it, vi } from "vitest";

import { useAgents } from "./useAgents";

vi.mock("@/components/networking", () => ({
  getAgentsList: vi.fn(),
}));

describe("useAgents", () => {
  it("does not call the agents API", () => {
    const result = useAgents();
    expect(result.data).toEqual({ agents: [] });
    expect(result.isLoading).toBe(false);
  });
});
