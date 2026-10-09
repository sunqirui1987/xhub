"use client";

import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import RoutingWorkspace from "@/components/routing_groups/RoutingWorkspace";

/** 路由管理入口读取当前凭证并呈现完整设置页；无写入副作用。 */
export default function RouteTemplatesPage() {
  const { accessToken } = useAuthorized();
  return <RoutingWorkspace accessToken={accessToken} />;
}
