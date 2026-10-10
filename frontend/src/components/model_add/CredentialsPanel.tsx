"use client";

import { Plus } from "lucide-react";
import { useState } from "react";

import { useCredentials } from "@/app/(dashboard)/hooks/credentials/useCredentials";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import {
  credentialCreateCall,
  credentialDeleteCall,
  CredentialItem,
  credentialUpdateCall,
} from "@/components/networking";
import { Button } from "@/components/ui/button";
import { stripMaskedSecrets } from "@/utils/maskedSecretUtils";
import { isProxyAdminRole } from "@/utils/roles";

import DeleteResourceModal from "../common_components/DeleteResourceModal";
import { toast } from "@/lib/toast";
import CredentialModal from "./CredentialModal";
import CredentialsTable from "./CredentialsTable";
import { useT } from "@/i18n";

const restrictedFields = ["credential_name", "custom_llm_provider", "provider_id", "catalog_id"];

/** 将表单 values 和已过滤的 credentialValues 组成持久化请求；返回凭据契约，新增与编辑共用，无副作用。 */
const buildCredential = (values: Record<string, unknown>, credentialValues: Record<string, unknown>) => ({
  credential_name: values.credential_name as string,
  credential_values: credentialValues,
  credential_info: {
    custom_llm_provider: values.custom_llm_provider as string,
    ...(typeof values.catalog_id === "string" ? { catalog_id: values.catalog_id.trim() } : {}),
    ...(typeof values.provider_id === "string" ? { provider_id: values.provider_id } : {}),
  },
});

/** 从 values 剔除名称与协议元数据；返回独立认证字典，避免将元数据作为供应商参数。 */
const withoutRestrictedFields = (values: Record<string, unknown>): Record<string, unknown> =>
  Object.fromEntries(Object.entries(values).filter(([key]) => !restrictedFields.includes(key)));

/** 管理供应商连接列表和增删改弹窗；无参数，返回按权限显示的界面，语言切换同步文案；保存或删除成功刷新列表。 */
export default function CredentialsPanel() {
  const t = useT();
  const { accessToken, userRole, isViewOnly } = useAuthorized();
  // Admin Viewer follows the read-parity rule: see credentials, do not modify.
  const canModifyCredentials = !isViewOnly && isProxyAdminRole(userRole ?? "");
  const { data: credentialsResponse, isLoading, refetch: refetchCredentials } = useCredentials();
  const credentialList = credentialsResponse?.credentials ?? [];

  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [isUpdateModalOpen, setIsUpdateModalOpen] = useState(false);
  const [selectedCredential, setSelectedCredential] = useState<CredentialItem | null>(null);
  const [credentialToDelete, setCredentialToDelete] = useState<CredentialItem | null>(null);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [isCredentialDeleting, setIsCredentialDeleting] = useState(false);

  /** 保存凭据；values 为表单参数，返回成功布尔值。失败保留弹窗草稿，成功关闭并刷新列表。 */
  const handleUpdateCredential = async (values: Record<string, unknown>) => {
    if (!accessToken) {
      return false;
    }
    try {
      const newCredential = buildCredential(values, stripMaskedSecrets(withoutRestrictedFields(values)));
      await credentialUpdateCall(accessToken, values.credential_name as string, newCredential);
      toast.success(t("Credential updated successfully"));
      setIsUpdateModalOpen(false);
      await refetchCredentials();
      return true;
    } catch {
      toast.error(t("Failed to update credential"));
      return false;
    }
  };

  /** 保存凭据；values 为表单参数，返回成功布尔值。失败保留弹窗草稿，成功关闭并刷新列表。 */
  const handleAddCredential = async (values: Record<string, unknown>) => {
    if (!accessToken) {
      return false;
    }
    try {
      const newCredential = buildCredential(values, withoutRestrictedFields(values));
      await credentialCreateCall(accessToken, newCredential);
      toast.success(t("Credential added successfully"));
      setIsAddModalOpen(false);
      await refetchCredentials();
      return true;
    } catch {
      toast.error(t("Failed to add credential"));
      return false;
    }
  };

  const handleDeleteCredential = async () => {
    if (!accessToken || !credentialToDelete) {
      return;
    }
    setIsCredentialDeleting(true);
    try {
      await credentialDeleteCall(accessToken, credentialToDelete.credential_name);
      toast.success(t("Credential deleted successfully"));
      await refetchCredentials();
    } catch (error) {
      toast.error(t("Failed to delete credential"));
    } finally {
      setCredentialToDelete(null);
      setIsDeleteModalOpen(false);
      setIsCredentialDeleting(false);
    }
  };

  const openEditModal = (credential: CredentialItem) => {
    setSelectedCredential(credential);
    setIsUpdateModalOpen(true);
  };

  const openDeleteModal = (credential: CredentialItem) => {
    setCredentialToDelete(credential);
    setIsDeleteModalOpen(true);
  };

  const closeDeleteModal = () => {
    setCredentialToDelete(null);
    setIsDeleteModalOpen(false);
  };

  return (
    <div className="mx-auto flex w-full flex-auto flex-col gap-4 overflow-y-auto p-2">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold text-foreground">{t("modelProviders.title")}</h2>
          <p className="text-sm text-muted-foreground">{t("modelProviders.listHint")}</p>
        </div>
        {canModifyCredentials && (
          <Button onClick={() => setIsAddModalOpen(true)}>
            <Plus className="size-4" />
            {t("modelProviders.add")}
          </Button>
        )}
      </div>

      <CredentialsTable
        credentials={credentialList}
        canModifyCredentials={canModifyCredentials}
        onEdit={openEditModal}
        onDelete={openDeleteModal}
        isLoading={isLoading}
      />

      {isAddModalOpen && (
        <CredentialModal
          mode="add"
          onSubmit={handleAddCredential}
          open={isAddModalOpen}
          onCancel={() => setIsAddModalOpen(false)}
        />
      )}

      {isUpdateModalOpen && (
        <CredentialModal
          mode="edit"
          open={isUpdateModalOpen}
          existingCredential={selectedCredential}
          onSubmit={handleUpdateCredential}
          onCancel={() => setIsUpdateModalOpen(false)}
        />
      )}

      <DeleteResourceModal
        isOpen={isDeleteModalOpen}
        onCancel={closeDeleteModal}
        onOk={handleDeleteCredential}
        title={t("Delete Credential?")}
        message={t(
          "Are you sure you want to delete this credential? This action cannot be undone and may break existing integrations.",
        )}
        resourceInformationTitle={t("Credential Information")}
        resourceInformation={[
          { label: t("Credential Name"), value: credentialToDelete?.credential_name },
          { label: t("Provider"), value: credentialToDelete?.credential_info?.custom_llm_provider || "-" },
        ]}
        confirmLoading={isCredentialDeleting}
        requiredConfirmation={credentialToDelete?.credential_name}
      />
    </div>
  );
}
