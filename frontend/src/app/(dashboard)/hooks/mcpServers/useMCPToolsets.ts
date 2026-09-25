import { MCPToolset } from "@/components/mcp_tools/types";

// MCP toolsets are not part of this gateway.
export const useMCPToolsets = () => {
  const data: MCPToolset[] = [];
  return { data, isLoading: false, isError: false, error: null, refetch: async () => ({ data }) };
};
