import type { SplitDeployment } from "./templateForm";

/** DeploymentIdentity 展示部署的提供商、上游型号、端点协议及稳定 ID。
 * 参数为目录条目和引用 ID；权重表单调用，缺失目录时提示失效引用，不暴露凭据。 */
export default function DeploymentIdentity({ deployment, id }: { deployment?: SplitDeployment; id: string }) {
  return (
    <span className="min-w-0 space-y-1">
      <span className="block break-all font-medium">
        {deployment
          ? [deployment.supplier || deployment.provider || "提供商未标注", deployment.model].join(" · ")
          : "部署已不存在"}
      </span>
      {deployment && (
        <span className="block break-all text-xs text-muted-foreground">
          {[deployment.model_name, deployment.transport, ...(deployment.endpoint_types ?? [])]
            .filter(Boolean)
            .join(" · ")}
        </span>
      )}
      <span className="block break-all font-mono text-xs text-muted-foreground">ID: {id}</span>
    </span>
  );
}
