import { parseAsString, useQueryState } from "nuqs";
import React, { useCallback, useEffect, useMemo, useState } from "react";

import BulkEditUserModal from "./BulkEditUsers";
import BulkCreateUsersButton from "@/components/bulk_create_users_button";
import { CreateUserButton } from "@/components/CreateUserButton";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

import {
  getPossibleUserRoles,
  getProxyBaseUrl,
  userListCall,
  UserListResponse,
} from "@/components/networking";
import SetPasswordModal from "@/components/SetPasswordModal";

import { DEBOUNCE_WAIT_MS } from "@/utils/debounceConstants";
import { accountRoleKind } from "@/utils/iamRoles";
import { isAdminRole, isProxyAdminRole } from "@/utils/roles";
import { t } from "@/i18n";
import { useDebouncedValue } from "@tanstack/react-pacer/debouncer";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
  RowSelectionState,
  SortingState,
} from "@tanstack/react-table";
import DeleteResourceModal from "@/components/common_components/DeleteResourceModal";
import { toast } from "@/lib/toast";
import { modelAvailableCall, userDeleteCall } from "@/components/networking";

import { UsersTable } from "./view_users/UsersTable";
import UserInfoView from "./view_users/user_info_view";
import { UserInfo } from "@/components/networking";

interface ViewUserDashboardProps {
  accessToken: string | null;
  token: string | null;
  userRole: string | null;
  userID: string | null;
  teams: any[] | null;
  orgAdminOrgIds?: Array<{ organization_id: string; organization_alias: string }> | null;
}

const DEFAULT_PAGE_SIZE = 25;

const DEFAULT_SORT_BY = "created_at";

const DEFAULT_SORTING: SortingState = [{ id: DEFAULT_SORT_BY, desc: true }];

const ViewUserDashboard: React.FC<ViewUserDashboardProps> = ({
  accessToken,
  token,
  userRole,
  userID,
  teams,
  orgAdminOrgIds,
}) => {
  const isProxyAdmin = userRole ? isProxyAdminRole(userRole) : false;
  const queryClient = useQueryClient();

  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: DEFAULT_PAGE_SIZE });
  const [sorting, setSorting] = useState<SortingState>(DEFAULT_SORTING);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [searchInput, setSearchInput] = useState("");
  const [searchQuery] = useDebouncedValue(searchInput, { wait: DEBOUNCE_WAIT_MS });

  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [selectionMode, setSelectionMode] = useState(false);
  const [isBulkEditModalVisible, setIsBulkEditModalVisible] = useState(false);

  const [selectedUserId, setSelectedUserId] = useQueryState("user", parseAsString.withOptions({ history: "push" }));
  const [openInEditMode, setOpenInEditMode] = useState(false);

  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [isDeletingUser, setIsDeletingUser] = useState(false);
  const [userToDelete, setUserToDelete] = useState<UserInfo | null>(null);
  const [isSetPasswordModalVisible, setIsSetPasswordModalVisible] = useState(false);
  const [passwordUser, setPasswordUser] = useState<UserInfo | null>(null);
  const [baseUrl, setBaseUrl] = useState<string | null>(null);
  const [userModels, setUserModels] = useState<string[]>([]);

  useEffect(() => {
    setBaseUrl(getProxyBaseUrl());
  }, []);

  // Fetch available models for bulk edit
  useEffect(() => {
    const fetchUserModels = async () => {
      try {
        if (!userID || !userRole || !accessToken) {
          return;
        }

        const model_available = await modelAvailableCall(accessToken, userID, userRole);
        let available_model_names = model_available["data"].map((element: { id: string }) => element.id);
        setUserModels(available_model_names);
      } catch (error) {
        console.error("Error fetching user models:", error);
      }
    };

    fetchUserModels();
  }, [accessToken, userID, userRole]);

  const getFilterValue = useCallback(
    (columnId: string): string | undefined => {
      const entry = columnFilters.find((filter) => filter.id === columnId);
      return typeof entry?.value === "string" && entry.value.trim() ? entry.value.trim() : undefined;
    },
    [columnFilters],
  );

  const handleSearchChange = useCallback((value: string) => {
    setSearchInput(value);
    setPagination((previous) => ({ ...previous, pageIndex: 0 }));
    setRowSelection({});
  }, []);

  const handleSortingChange = useCallback<OnChangeFn<SortingState>>((updaterOrValue) => {
    setSorting(updaterOrValue);
    setPagination((previous) => ({ ...previous, pageIndex: 0 }));
    setRowSelection({});
  }, []);

  const handleColumnFiltersChange = useCallback<OnChangeFn<ColumnFiltersState>>((updaterOrValue) => {
    setColumnFilters(updaterOrValue);
    setPagination((previous) => ({ ...previous, pageIndex: 0 }));
    setRowSelection({});
  }, []);

  const handlePaginationChange = useCallback<OnChangeFn<PaginationState>>((updaterOrValue) => {
    setPagination(updaterOrValue);
    setRowSelection({});
  }, []);

  const handleUserClick = useCallback(
    (userId: string, openInEdit: boolean = false) => {
      void setSelectedUserId(userId);
      setOpenInEditMode(openInEdit);
    },
    [setSelectedUserId],
  );

  /** 关闭详情并刷新用户目录；无参数和返回值，供详情返回按钮调用，使账号和团队编辑在列表立即可见。 */
  const handleCloseUserInfo = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["userList"] });
    void setSelectedUserId(null);
    setOpenInEditMode(false);
  }, [setSelectedUserId, queryClient]);

  const handleDelete = useCallback((user: UserInfo) => {
    setUserToDelete(user);
    setIsDeleteModalOpen(true);
  }, []);

  const handleResetPassword = useCallback((user: UserInfo) => {
    setPasswordUser(user);
    setIsSetPasswordModalVisible(true);
  }, []);

  const confirmDelete = async () => {
    if (userToDelete && accessToken) {
      try {
        setIsDeletingUser(true);
        await userDeleteCall(accessToken, [userToDelete.user_id]);

        // Update the user list after deletion
        queryClient.setQueriesData<UserListResponse>({ queryKey: ["userList"] }, (previousData) => {
          if (previousData === undefined) return previousData;
          const updatedUsers = previousData.users.filter((user) => user.user_id !== userToDelete.user_id);
          return { ...previousData, users: updatedUsers };
        });

        toast.success(t("User deleted successfully"));
      } catch (error) {
        console.error("Error deleting user:", error);
        toast.fromError(t("Failed to delete user"));
      } finally {
        setIsDeleteModalOpen(false);
        setUserToDelete(null);
        setIsDeletingUser(false);
      }
    }
  };

  const cancelDelete = () => {
    setIsDeleteModalOpen(false);
    setUserToDelete(null);
  };

  const handleToggleSelectionMode = () => {
    setSelectionMode(!selectionMode);
    setRowSelection({});
  };

  const handleBulkEditSuccess = () => {
    // Refresh the user list
    queryClient.invalidateQueries({ queryKey: ["userList"] });
    setRowSelection({});
    setSelectionMode(false);
  };

  const activeSort = sorting[0];
  const sortBy = activeSort?.id ?? DEFAULT_SORT_BY;
  const sortOrder: "asc" | "desc" = activeSort?.desc ?? true ? "desc" : "asc";

  const userIdFilter = getFilterValue("user_id");
  const ssoUserIdFilter = getFilterValue("sso_user_id");
  const userRoleFilter = getFilterValue("user_role");
  const teamFilter = getFilterValue("team");
  const searchFilter = searchQuery.trim() || null;

  const userListQueryFilters = {
    page: pagination.pageIndex + 1,
    pageSize: pagination.pageSize,
    search: searchFilter,
    userId: userIdFilter,
    ssoUserId: ssoUserIdFilter,
    role: userRoleFilter,
    team: teamFilter,
    sortBy,
    sortOrder,
    orgAdminOrgIds,
  };

  const userListQuery = useQuery({
    queryKey: ["userList", userListQueryFilters],
    queryFn: async () => {
      if (!accessToken) throw new Error(t("Access token required"));

      return await userListCall(
        accessToken,
        userIdFilter ? [userIdFilter] : null,
        pagination.pageIndex + 1,
        pagination.pageSize,
        null,
        userRoleFilter ?? null,
        teamFilter ?? null,
        ssoUserIdFilter ?? null,
        sortBy,
        sortOrder,
        orgAdminOrgIds ? orgAdminOrgIds.map((o) => o.organization_id) : null,
        searchFilter,
      );
    },
    enabled: Boolean(accessToken && token && userRole && userID),
    placeholderData: (previousData) => previousData,
  });

  const userRolesQuery = useQuery<Record<string, Record<string, string>>>({
    queryKey: ["userRoles"],
    initialData: () => ({}),
    queryFn: async () => {
      if (!accessToken) throw new Error(t("Access token required"));
      return await getPossibleUserRoles(accessToken);
    },
    enabled: Boolean(accessToken && token && userRole && userID),
  });
  const possibleUIRoles = userRolesQuery.data;

  const users = useMemo<UserInfo[]>(() => userListQuery.data?.users ?? [], [userListQuery.data]);
  const totalUserCount = userListQuery.data?.total ?? 0;

  const selectedUsers = useMemo(() => users.filter((user) => rowSelection[user.user_id]), [users, rowSelection]);

  if (selectedUserId) {
    return (
      <UserInfoView
        userId={selectedUserId}
        onClose={handleCloseUserInfo}
        accessToken={accessToken}
        userRole={userRole}
        possibleUIRoles={possibleUIRoles}
        initialTab={openInEditMode ? 1 : 0}
        startInEditMode={openInEditMode}
      />
    );
  }

  const usersTable = (
    <UsersTable
      data={users}
      rowCount={totalUserCount}
      isLoading={userListQuery.isLoading || userListQuery.isPlaceholderData}
      possibleUIRoles={possibleUIRoles}
      teams={teams}
      sorting={sorting}
      onSortingChange={handleSortingChange}
      pagination={pagination}
      onPaginationChange={handlePaginationChange}
      columnFilters={columnFilters}
      onColumnFiltersChange={handleColumnFiltersChange}
      searchValue={searchInput}
      onSearchChange={handleSearchChange}
      selectionEnabled={isProxyAdmin && selectionMode}
      rowSelection={rowSelection}
      onRowSelectionChange={setRowSelection}
      onUserClick={handleUserClick}
      onDeleteUser={handleDelete}
      onResetPassword={handleResetPassword}
    />
  );

  return (
    <div className="w-full overflow-hidden">
      <div className="mb-4 flex items-center justify-between">
        <div className="flex space-x-3">
          {userListQuery.isLoading && (
            <>
              <Skeleton className="h-9 w-28" />
              <Skeleton className="h-9 w-36" />
              <Skeleton className="h-9 w-28" />
            </>
          )}
          {!userListQuery.isLoading && userID && accessToken && (
            <>
              {isProxyAdmin && (
                <CreateUserButton userID={userID} accessToken={accessToken} possibleUIRoles={possibleUIRoles} />
              )}

              {isProxyAdmin && (
                <BulkCreateUsersButton accessToken={accessToken} teams={teams} possibleUIRoles={possibleUIRoles} />
              )}

              {isProxyAdmin && (
                <Button
                  type="button"
                  onClick={handleToggleSelectionMode}
                  variant={selectionMode ? "default" : "outline"}
                  data-testid="toggle-user-selection"
                >
                  {selectionMode ? t("pages.users.cancelSelection") : t("pages.users.selectUsers")}
                </Button>
              )}

              {isProxyAdmin && selectionMode && (
                <Button
                  type="button"
                  onClick={() => setIsBulkEditModalVisible(true)}
                  disabled={selectedUsers.length === 0}
                  data-testid="bulk-edit-users"
                >
                  {t("pages.users.bulkEditCount", { count: selectedUsers.length })}
                </Button>
              )}
            </>
          )}
        </div>
      </div>

      {usersTable}

      {/* Existing Modals */}
      <DeleteResourceModal
        isOpen={isDeleteModalOpen}
        title={t("pages.users.deleteTitle")}
        message={t("pages.users.deleteMessage")}
        resourceInformationTitle={t("pages.users.infoTitle")}
        resourceInformation={[
          { label: t("pages.users.email"), value: userToDelete?.user_email },
          { label: t("pages.users.userId"), value: userToDelete?.user_id, code: true },
          {
            label: t("pages.users.role"),
            value: (() => {
              const kind = accountRoleKind(userToDelete?.user_role);
              if (kind === "admin") return t("Platform administrator");
              if (kind === "user") return t("Regular user");
              return userToDelete?.user_role || "-";
            })(),
          },
          { label: t("pages.users.spend"), value: userToDelete?.spend?.toFixed(2) },
        ]}
        onCancel={cancelDelete}
        onOk={confirmDelete}
        confirmLoading={isDeletingUser}
      />

      <SetPasswordModal
        open={isSetPasswordModalVisible}
        onOpenChange={setIsSetPasswordModalVisible}
        accessToken={accessToken}
        user={passwordUser}
        onSuccess={() => queryClient.invalidateQueries({ queryKey: ["userList"] })}
      />

      <BulkEditUserModal
        open={isBulkEditModalVisible}
        onCancel={() => setIsBulkEditModalVisible(false)}
        selectedUsers={selectedUsers}
        possibleUIRoles={possibleUIRoles}
        accessToken={accessToken}
        onSuccess={handleBulkEditSuccess}
        teams={teams}
        userRole={userRole}
        userModels={userModels}
        allowAllUsers={userRole ? isAdminRole(userRole) : false}
      />
    </div>
  );
};

export default ViewUserDashboard;
