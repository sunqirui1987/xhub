"use client";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { ColumnWriteSurface } from "@/components/column_write_surface";
import { useMcpOAuthFlow } from "@/hooks/useMcpOAuthFlow";
function McpOAuthResume() {
  const { accessToken } = useAuthorized();
  const flow = useMcpOAuthFlow({ accessToken, getCredentials: () => undefined, getTemporaryPayload: () => null, onTokenReceived: () => undefined, flowSource: "e2e" });
  const text = flow.status === "success" ? JSON.stringify(flow.tokenResponse) : flow.error || "";
  return <p data-testid="oauth-resume">{text}</p>;
}
export default function McpServersPage() {
  return (<><McpOAuthResume /><ColumnWriteSurface title="MCP 服务器" emptyText="还没有 MCP 服务器" listPath="/server" actions={[
    { label: "创建", method: "POST", path: "/server", body: ({ name }) => ({ server_name: name, name }) },
    { label: "更新", method: "PUT", needsId: true, path: "/server", body: ({ name, id }) => ({ server_id: id, server_name: name, name }) },
    { label: "删除", method: "DELETE", needsId: true, path: ({ id }) => `/server/${id}` },
  ]} /></>);
}
