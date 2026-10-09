"use client";
import CredentialsPanel from "@/components/model_add/CredentialsPanel";
/** 模型提供商独立管理页，复用凭据组件的权限与保存流程；无重复连接存储。 */
export default function ModelProvidersPage(){return <div className="mx-auto w-full max-w-[1600px] space-y-6"><div className="rounded-xl border bg-card p-6"><h2 className="text-2xl font-semibold">模型提供商</h2><p className="text-sm text-muted-foreground">管理供应商连接、账号、地址与凭据。一个供应商可以有多个独立连接。</p></div><CredentialsPanel/></div>;}
