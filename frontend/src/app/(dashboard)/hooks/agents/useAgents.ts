import { AgentsResponse } from "@/components/agents/types";

// Agents are not part of this gateway. Callers keep a stable empty result and never hit /v1/agents.
export const useAgents = () => {
  const data: AgentsResponse = { agents: [] };
  return { data, isLoading: false, isError: false, error: null, refetch: async () => ({ data }) };
};
