"use client";
import CredentialsPanel from "@/components/model_add/CredentialsPanel";
import { useT } from "@/i18n";
/** 模型提供商独立管理页；无参数，返回随语言切换的标题及凭据管理组件，复用权限与保存流程，无重复存储。 */
export default function ModelProvidersPage() {
  const t = useT();
  return (
    <div className="mx-auto w-full max-w-[1600px] space-y-6">
      <div className="rounded-xl border bg-card p-6">
        <h2 className="text-2xl font-semibold">{t("modelProviders.title")}</h2>
        <p className="text-sm text-muted-foreground">{t("modelProviders.description")}</p>
      </div>
      <CredentialsPanel />
    </div>
  );
}
