"use client";
import RouteTemplatesPanel from "@/app/(dashboard)/route-templates/_components/RouteTemplatesPanel";
/** 路由入口仅呈现命名模板库；参数为管理凭证，所有组和回退编辑在模板内完成，无独立资源写入。 */
export default function RoutingWorkspace({accessToken}:{accessToken:string|null}) {
 return <RouteTemplatesPanel accessToken={accessToken}/>;
}
