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
import { ProviderModelDialog } from "./ProviderModelDialog";
import { t } from "@/i18n";

const DEFAULT_PROVIDERS: CredentialItem[] = [
  {
    credential_name: "fennoai",
    credential_values: { api_base: "https://api.fenno.ai" },
    credential_info: { custom_llm_provider: "openai", builtin: "fennoai", api_base: "https://api.fenno.ai", wire_api: "responses" },
  },
  {
    credential_name: "qiniu",
    credential_values: { api_base: "https://api.qnaigc.com/bypass/openai/v1" },
    credential_info: { custom_llm_provider: "openai", builtin: "qiniu", api_base: "https://api.qnaigc.com/bypass/openai/v1", wire_api: "responses" },
  },
];

const restrictedFields = ["credential_name", "custom_llm_provider"];

const buildCredential = (values: Record<string, unknown>, credentialValues: Record<string, unknown>) => ({
  credential_name: values.credential_name as string,
  credential_values: credentialValues,
  credential_info: {
    custom_llm_provider: values.custom_llm_provider as string,
  },
});

const withoutRestrictedFields = (values: Record<string, unknown>): Record<string, unknown> =>
  Object.fromEntries(Object.entries(values).filter(([key]) => !restrictedFields.includes(key)));

export default function CredentialsPanel() {
  const { accessToken, userRole } = useAuthorized();
  // Admin Viewer follows the read-parity rule: see credentials, do not modify.
  const canModifyCredentials = isProxyAdminRole(userRole ?? "");
  const { data: credentialsResponse, isLoading, refetch: refetchCredentials } = useCredentials();
  const credentialList = [
    ...DEFAULT_PROVIDERS.filter(
      (item) => !(credentialsResponse?.credentials || []).some((row) => row.credential_name === item.credential_name),
    ),
    ...(credentialsResponse?.credentials || []),
  ];

  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [listingProvider, setListingProvider] = useState<{ provider: "fennoai" | "qiniu"; credentialName: string } | null>(null);
  const [isUpdateModalOpen, setIsUpdateModalOpen] = useState(false);
  const [selectedCredential, setSelectedCredential] = useState<CredentialItem | null>(null);
  const [credentialToDelete, setCredentialToDelete] = useState<CredentialItem | null>(null);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [isCredentialDeleting, setIsCredentialDeleting] = useState(false);

  const handleUpdateCredential = async (values: Record<string, unknown>) => {
    if (!accessToken) {
      return;
    }
    try {
      const newCredential = buildCredential(values, stripMaskedSecrets(withoutRestrictedFields(values)));
      await credentialUpdateCall(accessToken, values.credential_name as string, newCredential);
      toast.success(t("Credential updated successfully"));
      setIsUpdateModalOpen(false);
      await refetchCredentials();
    } catch (error) {
      toast.error(t("Failed to update credential"));
    }
  };

  const handleAddCredential = async (values: Record<string, unknown>) => {
    if (!accessToken) {
      return;
    }
    try {
      const newCredential = buildCredential(values, withoutRestrictedFields(values));
      await credentialCreateCall(accessToken, newCredential);
      toast.success(t("Credential added successfully"));
      setIsAddModalOpen(false);
      await refetchCredentials();
    } catch (error) {
      toast.error(t("Failed to add credential"));
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
          <h2 className="text-lg font-semibold text-foreground">{t("pages.models.llmCredentials")}</h2>
          <p className="text-sm text-muted-foreground">
            {t("Configured credentials for different AI providers. Add and manage your API credentials.")}
          </p>
        </div>
        {canModifyCredentials && (
          <Button onClick={() => setIsAddModalOpen(true)}>
            <Plus className="size-4" />
            {t("Add Credential")}
          </Button>
        )}
      </div>

      <CredentialsTable
        credentials={credentialList}
        canModifyCredentials={canModifyCredentials}
        onEdit={openEditModal}
        onDelete={openDeleteModal}
        onListModels={(credential) => {
          const builtin = credential.credential_info?.builtin || credential.credential_name;
          if (builtin === "fennoai" || builtin === "qiniu") {
            setListingProvider({ provider: builtin, credentialName: credential.credential_name });
          }
        }}
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
      {listingProvider && accessToken ? (
        <ProviderModelDialog
          provider={listingProvider.provider}
          initialCredentialName={listingProvider.credentialName}
          credentials={credentialList}
          accessToken={accessToken}
          onClose={() => setListingProvider(null)}
          onAdded={() => {
            void refetchCredentials();
          }}
        />
      ) : null}

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
        message={t("Are you sure you want to delete this credential? This action cannot be undone and may break existing integrations.")}
        resourceInformationTitle="Credential Information"
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
