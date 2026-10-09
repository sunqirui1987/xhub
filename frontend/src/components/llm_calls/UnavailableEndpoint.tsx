import Link from "next/link";

/** UnavailableEndpoint 展示模型没有有效调用绑定时的原因和修正入口。
 * 参数 reason：后台配置校验原因；compare：是否仅缺少文本对比端点。
 * 返回：可访问的状态提示；用于 Playground/对比界面，不推断协议、不发送请求。 */
export function UnavailableEndpoint({ reason, compare = false }: { reason?: string; compare?: boolean }) {
  return (
    <div role="status" className="space-y-2 rounded-md border border-border bg-muted/30 p-4 text-sm">
      <p className="font-medium">{compare ? "该模型没有可用于对比的文本端点" : "该模型没有可调用的端点"}</p>
      <p>请在模型配置中明确选择接入端点和匹配的上游接口协议，保存后刷新模型列表。</p>
      {reason && <p className="break-words text-muted-foreground">{reason}</p>}
      <Link href="/models-and-endpoints" className="inline-block text-primary underline">前往模型配置</Link>
    </div>
  );
}
