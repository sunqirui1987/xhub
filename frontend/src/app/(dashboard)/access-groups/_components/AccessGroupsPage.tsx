import { AccessGroupResponse, useAccessGroups } from "@/app/(dashboard)/hooks/accessGroups/useAccessGroups";
import { useDeleteAccessGroup } from "@/app/(dashboard)/hooks/accessGroups/useDeleteAccessGroup";
import { Boxes, Plus, SearchIcon, X } from "lucide-react";
import { useMemo, useState } from "react";
import DeleteResourceModal from "@/components/common_components/DeleteResourceModal";
import { PageHeader } from "@/components/shared/PageHeader";
import { t } from "@/i18n";
import { Button } from "@/components/ui/button";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group";
import { AccessGroupDetail } from "./AccessGroupsDetailsPage";
import { AccessGroupCreateDialog } from "./access-group-create/AccessGroupCreateDialog";
import { AccessGroupsTable } from "./AccessGroupsTable";
import { AccessGroup } from "./types";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { isProxyAdminRole } from "@/utils/roles";

function mapResponseToAccessGroup(r: AccessGroupResponse): AccessGroup {
  return {
    id: r.access_group_id,
    name: r.access_group_name,
    description: r.description ?? "",
    modelIds: r.access_model_names,
    mcpServerIds: r.access_mcp_server_ids,
    agentIds: r.access_agent_ids,
    keyIds: r.assigned_key_ids,
    teamIds: r.assigned_team_ids,
    createdAt: r.created_at,
    createdBy: r.created_by ?? "",
    updatedAt: r.updated_at,
    updatedBy: r.updated_by ?? "",
  };
}

export function AccessGroupsPage() {
  const { userRole } = useAuthorized();
  // Admin Viewer follows the read-parity rule: see access groups, no writes.
  const canModify = isProxyAdminRole(userRole ?? "");
  const { data: groupsData, isLoading } = useAccessGroups();
  const groups = useMemo(() => (groupsData ?? []).map(mapResponseToAccessGroup), [groupsData]);

  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [isCreateModalVisible, setIsCreateModalVisible] = useState(false);
  const [searchText, setSearchText] = useState("");
  const [groupToDelete, setGroupToDelete] = useState<AccessGroup | null>(null);
  const deleteMutation = useDeleteAccessGroup();

  const filteredGroups = useMemo(() => {
    const query = searchText.trim().toLowerCase();
    if (!query) return groups;
    return groups.filter(
      (group) =>
        group.name.toLowerCase().includes(query) ||
        group.id.toLowerCase().includes(query) ||
        group.description.toLowerCase().includes(query),
    );
  }, [groups, searchText]);

  if (selectedGroupId) {
    return <AccessGroupDetail accessGroupId={selectedGroupId} onBack={() => setSelectedGroupId(null)} />;
  }

  return (
    <div className="p-8">
      <PageHeader
        icon={<Boxes />}
        title={t("pages.accessGroups.title")}
        subtitle={t("pages.accessGroups.subtitle")}
        primaryAction={
          canModify ? (
            <Button onClick={() => setIsCreateModalVisible(true)}>
              <Plus className="size-4" />
              {t("pages.accessGroups.create")}
            </Button>
          ) : undefined
        }
      />

      <div className="mt-6 mb-3 flex items-center">
        <InputGroup className="max-w-[400px]">
          <InputGroupAddon>
            <SearchIcon className="size-4 text-muted-foreground" />
          </InputGroupAddon>
          <InputGroupInput
            placeholder={t("pages.accessGroups.searchPlaceholder")}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
          />
          {searchText && (
            <InputGroupAddon align="inline-end">
              <InputGroupButton size="icon-xs" aria-label={t("pages.accessGroups.clearSearch")} onClick={() => setSearchText("")}>
                <X />
              </InputGroupButton>
            </InputGroupAddon>
          )}
        </InputGroup>
      </div>

      <AccessGroupsTable
        groups={filteredGroups}
        isLoading={isLoading}
        isFiltered={searchText.trim().length > 0}
        canModify={canModify}
        onGroupClick={setSelectedGroupId}
        onDeleteClick={setGroupToDelete}
      />

      <AccessGroupCreateDialog open={isCreateModalVisible} onOpenChange={setIsCreateModalVisible} />

      <DeleteResourceModal
        isOpen={!!groupToDelete}
        title={t("pages.accessGroups.deleteTitle")}
        message={t("pages.accessGroups.deleteMessage")}
        resourceInformationTitle={t("pages.accessGroups.infoTitle")}
        resourceInformation={[
          { label: t("common.id"), value: groupToDelete?.id, code: true },
          { label: t("common.name"), value: groupToDelete?.name },
          { label: t("common.description"), value: groupToDelete?.description || "—" },
        ]}
        onCancel={() => setGroupToDelete(null)}
        onOk={() => {
          if (!groupToDelete) return;
          deleteMutation.mutate(groupToDelete.id, {
            onSuccess: () => {
              setGroupToDelete(null);
            },
          });
        }}
        confirmLoading={deleteMutation.isPending}
      />
    </div>
  );
}
