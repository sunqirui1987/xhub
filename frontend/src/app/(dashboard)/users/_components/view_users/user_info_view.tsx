import React, { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  userGetInfoV2,
  UserInfoV2Response,
  userDeleteCall,
  teamInfoCall,
  teamListCall,
  teamMemberAddCall,
  teamMemberDeleteCall,
  organizationListCall,
  Member,
} from "@/components/networking";
import { SimpleTooltip } from "@/components/ui/tooltip";
import { accountRoleKind, isTeamAdminMembership } from "@/utils/iamRoles";
import { t } from "@/i18n";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { teamDetailHref } from "@/utils/entityLinks";
import { BadgeLink } from "@/components/shared/BadgeLink";
import { formatNumberWithCommas, copyToClipboard as utilCopyToClipboard } from "@/utils/dataUtils";
import { ArrowLeft, CheckIcon, CopyIcon, KeyRound, Plus, Trash2 } from "lucide-react";
import SetPasswordModal from "@/components/SetPasswordModal";
import { toast } from "@/lib/toast";
import { getBudgetDurationLabel } from "@/components/common_components/budget_duration_dropdown";
import DeleteResourceModal from "@/components/common_components/DeleteResourceModal";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";

interface UserInfoViewProps {
  userId: string;
  onClose: () => void;
  accessToken: string | null;
  userRole: string | null;
  onDelete?: () => void;
  possibleUIRoles: Record<string, Record<string, string>> | null;
  initialTab?: number; // 0 for Overview, 1 for Details
  startInEditMode?: boolean;
}

/** Team info used for display in user detail view */
interface TeamDisplayInfo {
  team_id: string;
  team_alias: string | null;
  user_role?: string | null;
  organization_alias?: string | null;
}

const accountRoleLabel = (role: string | null | undefined): string => {
  const kind = accountRoleKind(role);
  if (kind === "admin") return t("Platform administrator");
  if (kind === "user") return t("Regular user");
  if (!role) return t("pages.users.notSet");
  return role;
};

const membershipRoleLabel = (role: string | null | undefined): string => {
  if (!role) return "—";
  return isTeamAdminMembership(role) ? t("Team admin") : t("Team member");
};

const unwrapUserInfo = (data: unknown): UserInfoV2Response | null => {
  if (!data || typeof data !== "object") return null;
  const record = data as { user_id?: string; user_info?: UserInfoV2Response };
  if (record.user_info && !record.user_id) return record.user_info;
  return data as UserInfoV2Response;
};

const asTeamRows = (raw: unknown): Array<Record<string, any>> => {
  if (Array.isArray(raw)) return raw;
  if (raw && typeof raw === "object" && Array.isArray((raw as { teams?: unknown }).teams)) {
    return (raw as { teams: Array<Record<string, any>> }).teams;
  }
  return [];
};

const organizationNames = async (accessToken: string): Promise<Map<string, string>> => {
  const names = new Map<string, string>();
  try {
    const raw = await organizationListCall(accessToken);
    const rows = Array.isArray(raw) ? raw : ((raw as { organizations?: unknown[] })?.organizations ?? []);
    for (const row of rows as Array<Record<string, string>>) {
      if (row.organization_id) {
        names.set(row.organization_id, row.organization_alias || row.organization_name || row.organization_id);
      }
    }
  } catch {
    // The organization name is a label. The team still shows without it.
  }
  return names;
};

const resolveInfoTeams = async (accessToken: string, infoTeams: unknown): Promise<TeamDisplayInfo[]> => {
  if (!Array.isArray(infoTeams) || infoTeams.length === 0) return [];
  return Promise.all(
    infoTeams.map(async (item) => {
      if (typeof item === "string") {
        try {
          const teamData = await teamInfoCall(accessToken, item);
          return {
            team_id: item,
            team_alias: teamData?.team_info?.team_alias || teamData?.team_alias || null,
            user_role: null,
            organization_alias: null,
          };
        } catch {
          return { team_id: item, team_alias: null, user_role: null, organization_alias: null };
        }
      }
      const team = item as Record<string, any>;
      return {
        team_id: String(team.team_id ?? ""),
        team_alias: team.team_alias ?? null,
        user_role: team.user_role ?? null,
        organization_alias: team.organization_alias ?? null,
      };
    }),
  );
};

// The user-info payload often carries an empty teams list. The team list for
// this account is the membership, including which organization owns the team.
const loadUserTeams = async (accessToken: string, userId: string, infoTeams: unknown): Promise<TeamDisplayInfo[]> => {
  try {
    const rows = asTeamRows(await teamListCall(accessToken, null, userId));
    if (rows.length > 0) {
      const orgs = await organizationNames(accessToken);
      return rows.map((team) => ({
        team_id: team.team_id,
        team_alias: team.team_alias || team.team_id,
        user_role: team.user_role ?? null,
        organization_alias: orgs.get(team.organization_id) ?? team.organization_alias ?? null,
      }));
    }
  } catch {
    // Fall through to whatever the user payload named.
  }
  return resolveInfoTeams(accessToken, infoTeams);
};

const ADD_TEAM_FIELD_ID = "add-team-team";
const ADD_TEAM_ROLE_FIELD_ID = "add-team-role";

interface TeamOption {
  team_id: string;
  team_alias: string;
}

const memberRoleOptions = () => [
  { value: "user", label: t("Team member"), hint: t("Can view team info, but not manage it") },
  { value: "admin", label: t("Team admin"), hint: t("Can create team keys, add members, and manage settings") },
];

export default function UserInfoView({
  userId,
  onClose,
  accessToken,
  userRole,
  onDelete,
  initialTab = 0,
}: UserInfoViewProps) {
  const [userData, setUserData] = useState<UserInfoV2Response | null>(null);
  const [teamDetails, setTeamDetails] = useState<TeamDisplayInfo[]>([]);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [isSetPasswordOpen, setIsSetPasswordOpen] = useState(false);
  const [isDeletingUser, setIsDeletingUser] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<string>(initialTab === 1 ? "details" : "overview");
  const [copiedStates, setCopiedStates] = useState<Record<string, boolean>>({});
  const [isTeamsExpanded, setIsTeamsExpanded] = useState(false);
  const [isAddTeamModalOpen, setIsAddTeamModalOpen] = useState(false);
  const [isRemoveTeamModalOpen, setIsRemoveTeamModalOpen] = useState(false);
  const [teamToRemove, setTeamToRemove] = useState<TeamDisplayInfo | null>(null);
  const [isAddingTeam, setIsAddingTeam] = useState(false);
  const [isRemovingTeam, setIsRemovingTeam] = useState(false);
  const [allTeams, setAllTeams] = useState<Array<{ team_id: string; team_alias: string }>>([]);
  const [selectedTeamId, setSelectedTeamId] = useState<string>("");
  const [selectedRole, setSelectedRole] = useState<string>("user");
  const [isLoadingTeams, setIsLoadingTeams] = useState(false);


  React.useEffect(() => {
    const fetchData = async () => {
      try {
        if (!accessToken) return;
        const data = unwrapUserInfo(await userGetInfoV2(accessToken, userId));
        setUserData(data);
        if (data) setTeamDetails(await loadUserTeams(accessToken, userId, data.teams));
      } catch (error) {
        console.error("Error fetching user data:", error);
        toast.fromError(t("Failed to fetch user data"));
      } finally {
        setIsLoading(false);
      }
    };

    fetchData();
  }, [accessToken, userId, userRole]);

  const isProxyAdmin = userRole === "proxy_admin" || userRole === "Admin";

  const fetchAllTeams = async () => {
    if (!accessToken) return;
    setIsLoadingTeams(true);
    try {
      const teams = await teamListCall(accessToken, null);
      setAllTeams(
        (teams || []).map((t: any) => ({
          team_id: t.team_id,
          team_alias: t.team_alias || t.team_id,
        })),
      );
    } catch (error) {
      console.error("Error fetching teams:", error);
    } finally {
      setIsLoadingTeams(false);
    }
  };

  const handleOpenAddTeamModal = () => {
    setSelectedTeamId("");
    setSelectedRole("user");
    setIsAddTeamModalOpen(true);
    fetchAllTeams();
  };

  const handleAddTeamSubmit = async () => {
    if (!accessToken || !selectedTeamId) return;
    setIsAddingTeam(true);
    try {
      const member: Member = {
        role: selectedRole,
        user_id: userId,
        user_email: userData?.user_email ?? null,
      };
      await teamMemberAddCall(accessToken, selectedTeamId, member);
      toast.success(t("User added to team successfully"));
      setIsAddTeamModalOpen(false);
      const data = unwrapUserInfo(await userGetInfoV2(accessToken, userId));
      setUserData(data);
      setTeamDetails(await loadUserTeams(accessToken, userId, data?.teams));
    } catch (error: any) {
      console.error("Error adding user to team:", error);
      toast.fromError(error?.message || t("Failed to add user to team"));
    } finally {
      setIsAddingTeam(false);
    }
  };

  const handleOpenRemoveTeamModal = (team: TeamDisplayInfo) => {
    setTeamToRemove(team);
    setIsRemoveTeamModalOpen(true);
  };

  const handleRemoveTeamConfirm = async () => {
    if (!accessToken || !teamToRemove) return;
    setIsRemovingTeam(true);
    try {
      const member: Member = {
        role: "user",
        user_id: userId,
      };
      await teamMemberDeleteCall(accessToken, teamToRemove.team_id, member);
      toast.success(t("User removed from team successfully"));
      setIsRemoveTeamModalOpen(false);
      setTeamToRemove(null);
      // Re-fetch user data to refresh teams
      const data = unwrapUserInfo(await userGetInfoV2(accessToken, userId));
      setUserData(data);
      setTeamDetails(await loadUserTeams(accessToken, userId, data?.teams));
    } catch (error: any) {
      console.error("Error removing user from team:", error);
      toast.fromError(error?.message || t("Failed to remove user from team"));
    } finally {
      setIsRemovingTeam(false);
    }
  };

  const handleRemoveTeamCancel = () => {
    setIsRemoveTeamModalOpen(false);
    setTeamToRemove(null);
  };

  const availableTeamsForAdd = allTeams.filter((t) => !teamDetails.some((td) => td.team_id === t.team_id));

  const selectedTeamOption = availableTeamsForAdd.find((team) => team.team_id === selectedTeamId) ?? null;

  const handleDelete = async () => {
    try {
      if (!accessToken) return;
      setIsDeletingUser(true);
      await userDeleteCall(accessToken, [userId]);
      toast.success(t("User deleted successfully"));
      if (onDelete) {
        onDelete();
      }
      onClose();
    } catch (error) {
      console.error("Error deleting user:", error);
      toast.fromError(t("Failed to delete user"));
    } finally {
      setIsDeleteModalOpen(false);
      setIsDeletingUser(false);
    }
  };

  const cancelDelete = () => {
    setIsDeleteModalOpen(false);
  };

  if (isLoading) {
    return (
      <div className="p-4">
        <Button variant="ghost" onClick={onClose} className="mb-4">
          <ArrowLeft />
          {t("Back to Users")}
        </Button>
        <p className="text-sm">{t("pages.users.loadingUser")}</p>
      </div>
    );
  }

  if (!userData) {
    return (
      <div className="p-4">
        <Button variant="ghost" onClick={onClose} className="mb-4">
          <ArrowLeft />
          {t("Back to Users")}
        </Button>
        <p className="text-sm">{t("pages.users.userNotFound")}</p>
      </div>
    );
  }

  const copyToClipboard = async (text: string, key: string) => {
    const success = await utilCopyToClipboard(text);
    if (success) {
      setCopiedStates((prev) => ({ ...prev, [key]: true }));
      setTimeout(() => {
        setCopiedStates((prev) => ({ ...prev, [key]: false }));
      }, 2000);
    }
  };

  return (
    <div className="p-4">
      <div className="flex justify-between items-center mb-6">
        <div>
          <Button variant="ghost" onClick={onClose} className="mb-4">
            <ArrowLeft />
            {t("Back to Users")}
          </Button>
          <h2 className="text-xl font-semibold">{userData.user_email || t("User")}</h2>
          <div className="flex items-center cursor-pointer">
            <span className="text-sm text-muted-foreground font-mono">{userData.user_id}</span>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => copyToClipboard(userData.user_id, "user-id")}
              className={`left-2 z-raised transition-all duration-200 ${
                copiedStates["user-id"]
                  ? "text-success bg-success/10 border-success/20"
                  : "text-muted-foreground hover:text-foreground hover:bg-accent"
              }`}
            >
              {copiedStates["user-id"] ? <CheckIcon size={12} /> : <CopyIcon size={12} />}
            </Button>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <Button variant="secondary" onClick={() => setIsSetPasswordOpen(true)}>
            <KeyRound />
            {t("pages.users.resetPassword")}
          </Button>
          {isProxyAdmin && (
            <Button
              variant="secondary"
              onClick={() => setIsDeleteModalOpen(true)}
              className="flex items-center text-destructive border-destructive hover:bg-destructive/10"
            >
              <Trash2 />
              {t("pages.users.deleteUser")}
            </Button>
          )}
        </div>
      </div>

      <DeleteResourceModal
        isOpen={isDeleteModalOpen}
        title={t("pages.users.deleteTitle")}
        message={t("pages.users.deleteMessage")}
        resourceInformationTitle={t("pages.users.infoTitle")}
        resourceInformation={[
          { label: t("pages.users.email"), value: userData.user_email },
          { label: t("pages.users.userId"), value: userData.user_id, code: true },
          {
            label: t("pages.users.role"),
            value: accountRoleLabel(userData.user_role),
          },
          {
            label: t("pages.users.spend"),
            value: userData.spend !== null && userData.spend !== undefined ? userData.spend.toFixed(2) : undefined,
          },
        ]}
        onCancel={cancelDelete}
        onOk={handleDelete}
        confirmLoading={isDeletingUser}
      />

      <Tabs value={activeTab} onValueChange={(v: unknown) => setActiveTab(String(v))} className="gap-0">
        <TabsList variant="line" className="mb-4">
          <TabsTrigger value="overview" className="flex-none data-active:text-primary after:bg-primary">
            {t("common.overview")}
          </TabsTrigger>
          <TabsTrigger value="details" className="flex-none data-active:text-primary after:bg-primary">
            {t("common.details")}
          </TabsTrigger>
        </TabsList>

        {/* Overview Panel */}
        <TabsContent value="overview" keepMounted>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
            <Card className="block p-6">
              <p>{t("pages.users.spendUsd")}</p>
              <div className="mt-2">
                <h3 className="text-lg font-medium">${formatNumberWithCommas(userData.spend || 0, 2)}</h3>
                <p>
                  {t("pages.users.spendOf", {
                    limit:
                      userData.max_budget !== null
                        ? `$${formatNumberWithCommas(userData.max_budget, 2)}`
                        : t("pages.users.unlimited"),
                  })}
                </p>
              </div>
            </Card>

            <Card className="block p-6">
              <div className="flex justify-between items-center mb-2">
                <div>
                  <p>{t("common.teams")}</p>
                  <p className="text-xs text-muted-foreground">
                    {t("There is no organization administrator. Each team belongs to one organization.")}
                  </p>
                </div>
                {isProxyAdmin && (
                  <Button variant="ghost" size="sm" onClick={handleOpenAddTeamModal}>
                    <Plus />
                    {t("pages.users.addTeam")}
                  </Button>
                )}
              </div>
              <div className="mt-2">
                {teamDetails.length > 0 ? (
                  <div className="max-h-60 overflow-y-auto">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>{t("pages.users.teamName")}</TableHead>
                          <TableHead>{t("Organization")}</TableHead>
                          <TableHead>{t("Role in this team")}</TableHead>
                          {isProxyAdmin && <TableHead className="text-right">{t("common.actions")}</TableHead>}
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {teamDetails.slice(0, isTeamsExpanded ? teamDetails.length : 20).map((team) => (
                          <TableRow key={team.team_id}>
                            <TableCell>
                              <BadgeLink href={teamDetailHref(team.team_id)}>
                                {team.team_alias || team.team_id}
                              </BadgeLink>
                            </TableCell>
                            <TableCell>{team.organization_alias || "—"}</TableCell>
                            <TableCell>{membershipRoleLabel(team.user_role)}</TableCell>
                            {isProxyAdmin && (
                              <TableCell className="text-right">
                                <Button
                                  variant="ghost"
                                  size="icon-sm"
                                  aria-label={t("Remove from {value0}", { value0: (team.team_alias || team.team_id) })}
                                  onClick={() => handleOpenRemoveTeamModal(team)}
                                  className="text-destructive"
                                >
                                  <Trash2 />
                                </Button>
                              </TableCell>
                            )}
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </div>
                ) : (
                  <p>{t("pages.users.noTeams")}</p>
                )}
                {!isTeamsExpanded && teamDetails.length > 20 && (
                  <Button variant="ghost" size="sm" className="mt-2" onClick={() => setIsTeamsExpanded(true)}>
                    {t("pages.users.moreCount", { n: teamDetails.length - 20 })}
                  </Button>
                )}
                {isTeamsExpanded && teamDetails.length > 20 && (
                  <Button variant="ghost" size="sm" className="mt-2" onClick={() => setIsTeamsExpanded(false)}>
                    {t("Show Less")}
                  </Button>
                )}
              </div>
            </Card>

            <Card className="block p-6">
              <p>{t("pages.users.personalModels")}</p>
              <div className="mt-2">
                {userData.models?.length && userData.models?.length > 0 ? (
                  userData.models?.map((model, index) => <p key={index}>{model}</p>)
                ) : (
                  <p>{t("Models are decided by the teams below. This account has no model list of its own.")}</p>
                )}
              </div>
            </Card>
          </div>
        </TabsContent>

        {/* Details Panel */}
        <TabsContent value="details" keepMounted>
          <Card className="block p-6">
            <div className="flex justify-between items-center mb-4">
              <h3 className="text-lg font-medium">{t("pages.users.userSettings")}</h3>
            </div>

            <div className="space-y-4">
                <div>
                  <p className="font-medium">{t("pages.users.userId")}</p>
                  <div className="flex items-center cursor-pointer">
                    <span className="font-mono">{userData.user_id}</span>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => copyToClipboard(userData.user_id, "user-id")}
                      className={`left-2 z-raised transition-all duration-200 ${
                        copiedStates["user-id"]
                          ? "text-success bg-success/10 border-success/20"
                          : "text-muted-foreground hover:text-foreground hover:bg-accent"
                      }`}
                    >
                      {copiedStates["user-id"] ? <CheckIcon size={12} /> : <CopyIcon size={12} />}
                    </Button>
                  </div>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.email")}</p>
                  <p>{userData.user_email || t("pages.users.notSet")}</p>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.alias")}</p>
                  <p>{userData.user_alias || t("pages.users.notSet")}</p>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.role")}</p>
                  <p>{accountRoleLabel(userData.user_role)}</p>
                </div>

                <div>
                  <p className="font-medium">{t("common.created")}</p>
                  <p>{userData.created_at ? new Date(userData.created_at).toLocaleString() : t("common.unknown")}</p>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.lastUpdated")}</p>
                  <p>{userData.updated_at ? new Date(userData.updated_at).toLocaleString() : t("common.unknown")}</p>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.personalModels")}</p>
                  <div className="flex flex-wrap gap-2 mt-1">
                    {userData.models?.length && userData.models?.length > 0 ? (
                      userData.models?.map((model, index) => (
                        <span key={index} className="px-2 py-1 bg-info/15 rounded-sm text-xs">
                          {model}
                        </span>
                      ))
                    ) : (
                      <p>{t("Models are decided by the teams below. This account has no model list of its own.")}</p>
                    )}
                  </div>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.maxBudget")}</p>
                  <p>
                    {userData.max_budget !== null && userData.max_budget !== undefined
                      ? `$${formatNumberWithCommas(userData.max_budget, 4)}`
                      : t("pages.users.unlimited")}
                  </p>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.budgetReset")}</p>
                  <p>{getBudgetDurationLabel(userData.budget_duration ?? null)}</p>
                </div>

                <div>
                  <p className="font-medium">{t("pages.users.metadata")}</p>
                  <pre className="bg-muted p-2 rounded-sm text-xs overflow-auto mt-1">
                    {JSON.stringify(userData.metadata || {}, null, 2)}
                  </pre>
                </div>
            </div>
          </Card>
        </TabsContent>
      </Tabs>
      <SetPasswordModal
        open={isSetPasswordOpen}
        onOpenChange={setIsSetPasswordOpen}
        accessToken={accessToken}
        user={userData ? { user_id: userId, user_email: userData.user_email, user_alias: userData.user_alias } : null}
      />

      {/* Delete Team Member Modal */}
      <DeleteResourceModal
        isOpen={isRemoveTeamModalOpen}
        title={t("pages.users.removeFromTeam")}
        alertMessage={t("pages.users.removeFromTeamAlert")}
        message={t("pages.users.removeFromTeamMessage")}
        resourceInformationTitle={t("pages.users.teamMembership")}
        resourceInformation={[
          { label: t("pages.users.team"), value: teamToRemove?.team_alias || teamToRemove?.team_id },
          { label: t("pages.users.userId"), value: userData?.user_id, code: true },
          { label: t("pages.users.email"), value: userData?.user_email },
        ]}
        onCancel={handleRemoveTeamCancel}
        onOk={handleRemoveTeamConfirm}
        confirmLoading={isRemovingTeam}
      />

      {/* Add to Team Modal */}
      <Dialog
        open={isAddTeamModalOpen}
        onOpenChange={(open) => !open && setIsAddTeamModalOpen(false)}
        disablePointerDismissal={isAddingTeam}
      >
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle>{t("pages.users.addUserToTeam")}</DialogTitle>
          </DialogHeader>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              handleAddTeamSubmit();
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor={ADD_TEAM_FIELD_ID}>{t("pages.users.team")}</FieldLabel>
                <Combobox
                  items={availableTeamsForAdd}
                  value={selectedTeamOption}
                  onValueChange={(team: TeamOption | null) => setSelectedTeamId(team?.team_id ?? "")}
                  itemToStringLabel={(team: TeamOption) => team.team_alias}
                  isItemEqualToValue={(team: TeamOption, value: TeamOption) => team.team_id === value.team_id}
                >
                  <ComboboxInput id={ADD_TEAM_FIELD_ID} placeholder={t("pages.users.selectTeam")} className="w-full" />
                  <ComboboxContent>
                    <ComboboxEmpty>{t("pages.users.noTeamsFound")}</ComboboxEmpty>
                    <ComboboxList>
                      {(team: TeamOption) => (
                        <ComboboxItem key={team.team_id} value={team} title={team.team_alias}>
                          {team.team_alias}
                        </ComboboxItem>
                      )}
                    </ComboboxList>
                  </ComboboxContent>
                </Combobox>
              </Field>

              <Field>
                <FieldLabel htmlFor={ADD_TEAM_ROLE_FIELD_ID}>{t("pages.users.memberRole")}</FieldLabel>
                <Select
                  items={memberRoleOptions()}
                  value={selectedRole}
                  onValueChange={(value) => value !== null && setSelectedRole(value)}
                >
                  <SelectTrigger id={ADD_TEAM_ROLE_FIELD_ID} className="w-full">
                    <SelectValue>
                      {(value) => memberRoleOptions().find((option) => option.value === value)?.label ?? value}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {memberRoleOptions().map((option) => (
                      <SelectItem key={option.value} value={option.value} title={option.label}>
                        <SimpleTooltip content={option.hint}>
                          <span className="font-medium">{option.label}</span>
                          <span className="ml-2 text-muted-foreground text-sm">- {option.hint}</span>
                        </SimpleTooltip>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </FieldGroup>

            <div className="text-right mt-4">
              <Button type="submit" disabled={isAddingTeam || !selectedTeamId} aria-busy={isAddingTeam}>
                {isAddingTeam ? t("pages.users.adding") : t("pages.users.addToTeam")}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
