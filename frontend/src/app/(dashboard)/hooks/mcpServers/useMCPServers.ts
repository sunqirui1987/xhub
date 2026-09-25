import { MCPServer } from "@/components/mcp_tools/types";

// MCP servers are not part of this gateway. Callers keep an empty list and never hit the MCP API.
export const useMCPServers = (_teamId?: string | null) => {
  const data: MCPServer[] = [];
  return { data, isLoading: false, isError: false, error: null, refetch: async () => ({ data }) };
};
