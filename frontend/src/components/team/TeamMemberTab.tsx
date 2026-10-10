import { useUISettings } from "@/app/(dashboard)/hooks/uiSettings/useUISettings";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { SimpleTooltip } from "@/components/ui/tooltip";
import MemberTable from "@/components/common_components/MemberTable";
import { Member } from "@/components/networking";
import { MoneyCell } from "@/components/shared/table_cells";
import { formatNumberWithCommas } from "@/utils/dataUtils";
import { isProxyAdminRole, isUserTeamAdminForSingleTeam } from "@/utils/roles";
import { CircleHelp } from "lucide-react";
import type { ComponentProps } from "react";
import { TeamData } from "./TeamInfo";
import { t } from "@/i18n";

interface TeamMemberTabProps {
  teamData: TeamData;
  canEditTeam: boolean;
  handleMemberDelete: (member: Member) => void;
  setSelectedEditMember: (member: Member) => void;
  setIsEditMemberModalVisible: (visible: boolean) => void;
  setIsAddMemberModalVisible: (visible: boolean) => void;
}

/** 展示唯一团队成员的个人累计消费、固定或共享额度及模型权限。
 * 参数包含团队数据、编辑权限及成员操作回调；返回成员表格。
 * 调用：团队详情页；退出团队后个人累计账单保留，界面不承诺自动周期重置。
 */
export default function TeamMemberTab({
  teamData,
  canEditTeam,
  handleMemberDelete,
  setSelectedEditMember,
  setIsEditMemberModalVisible,
  setIsAddMemberModalVisible,
}: TeamMemberTabProps) {
  /** 格式化限速数字；参数为可能为空的金额/次数，返回去除多余零的字符串，无副作用。 */
  const formatNumber = (value: number | null): string => {
    if (value === null || value === undefined) return "0";

    if (typeof value === "number") {
      // Convert scientific notation to normal decimal
      const normalNumber = Number(value);

      // If it's a whole number, return it without decimals
      if (normalNumber === Math.floor(normalNumber)) {
        return normalNumber.toString();
      }

      // For decimal numbers, use toFixed and remove trailing zeros
      return formatNumberWithCommas(normalNumber, 8).replace(/\.?0+$/, "");
    }

    return "0";
  };

  /** 按成员 ID 读取个人累计消费；缺失成员返回零，供消费列和排序使用。 */
  const getUserSpend = (userId: string | null): number => {
    if (!userId) return 0;
    const membership = teamData.team_memberships.find((tm) => tm.user_id === userId);
    return membership?.spend ?? 0;
  };

  /** 按成员 ID 读取个人上限；返回 null 表示共享团队余额，供额度列使用。 */
  const getUserBudget = (userId: string | null): number | null => {
    if (!userId) return null;
    const membership = teamData.team_memberships.find((tm) => tm.user_id === userId);
    return membership?.litellm_budget_table?.max_budget ?? null;
  };

  /** 按成员 ID 读取并格式化限速；返回本地化共享提示或 RPM/TPM，无副作用。 */
  const getUserRateLimits = (userId: string | null): string => {
    if (!userId) return t("quotaGuide.rateBlank");
    const membership = teamData.team_memberships.find((tm) => tm.user_id === userId);
    const rpmLimit = membership?.litellm_budget_table?.rpm_limit;
    const tpmLimit = membership?.litellm_budget_table?.tpm_limit;

    const rpmText = rpmLimit != null ? `${formatNumber(rpmLimit)} RPM` : null;
    const tpmText = tpmLimit != null ? `${formatNumber(tpmLimit)} TPM` : null;

    const limits = [rpmText, tpmText].filter(Boolean);
    return limits.length > 0 ? limits.join(" / ") : t("quotaGuide.rateBlank");
  };

  const { data: uiSettingsData } = useUISettings();
  const { userId, userRole } = useAuthorized();
  const disableTeamAdminDeleteTeamUser = Boolean(uiSettingsData?.values?.disable_team_admin_delete_team_user);
  const isUserTeamAdmin = isUserTeamAdminForSingleTeam(teamData.team_info.members_with_roles, userId || "");
  const isProxyAdmin = isProxyAdminRole(userRole || "");

  /** 按成员 ID 读取模型白名单；缺失或空列表返回 null，表示继承团队模型范围。 */
  const getUserAllowedModels = (userId: string | null): string[] | null => {
    if (!userId) return null;
    const membership = teamData.team_memberships.find((tm) => tm.user_id === userId);
    const models = membership?.litellm_budget_table?.allowed_models;
    return models && models.length > 0 ? models : null;
  };

  const extraColumns: NonNullable<ComponentProps<typeof MemberTable>["extraColumns"]> = [
    {
      title: (
        <span className="flex items-center gap-1">
          {t("Model Scope")}
          <SimpleTooltip content={t("Models this member can access. Empty means they inherit all team models.")}>
            <CircleHelp className="size-4" aria-label={t("Model scope information")} />
          </SimpleTooltip>
        </span>
      ),
      key: "model_scope",
      render: (record: Member) => {
        const models = getUserAllowedModels(record.user_id);
        if (!models) {
          return <span className="text-muted-foreground">{t("(all team models)")}</span>;
        }
        const displayed = models.slice(0, 2);
        const remaining = models.length - displayed.length;
        return (
          <div className="flex flex-wrap gap-1">
            {displayed.map((m) => (
              <code key={m} className="rounded bg-muted px-1 py-0.5 text-xs">
                {m}
              </code>
            ))}
            {remaining > 0 && (
              <SimpleTooltip content={models.slice(2).join(", ")}>
                <span className="text-muted-foreground">{t("quotaGuide.moreModels", { count: remaining })}</span>
              </SimpleTooltip>
            )}
          </div>
        );
      },
    },
    {
      title: (
        <span className="flex items-center gap-1">
          {t("quotaGuide.spend")}
          <SimpleTooltip content={t("quotaGuide.spendHelp")}>
            <CircleHelp className="size-4" aria-label={t("quotaGuide.spend")} />
          </SimpleTooltip>
        </span>
      ),
      key: "spend",
      sortValue: (record: Member) => getUserSpend(record.user_id),
      render: (record: Member) => <MoneyCell value={getUserSpend(record.user_id)} decimals={2} />,
    },
    {
      title: t("Team Member Budget (USD)"),
      key: "budget",
      sortValue: (record: Member) => getUserBudget(record.user_id),
      render: (record: Member) => (
        <MoneyCell value={getUserBudget(record.user_id)} decimals={2} emptyText={t("quotaGuide.blank")} showZero />
      ),
    },
    {
      title: (
        <span className="flex items-center gap-1">
          {t("Team Member Rate Limits")}
          <SimpleTooltip content={t("Rate limits for this member's usage within this team.")}>
            <CircleHelp className="size-4" aria-label={t("Team member rate limits information")} />
          </SimpleTooltip>
        </span>
      ),
      key: "rate_limits",
      render: (record: Member) => <span>{getUserRateLimits(record.user_id)}</span>,
    },
  ];

  const hasMemberBudgets = teamData.team_memberships.some(
    (membership) =>
      membership.litellm_budget_table != null || membership.spend != null || membership.total_spend != null,
  );

  return (
    <MemberTable
      key={teamData.team_id}
      members={teamData.team_info.members_with_roles}
      canEdit={canEditTeam}
      onEdit={(record) => {
        const membership = teamData.team_memberships.find((tm) => tm.user_id === record.user_id);
        const normalized = (record.role || "").toLowerCase();
        const enhancedMember = {
          ...record,
          role: normalized === "admin" || normalized === "team_admin" ? "admin" : "user",
          max_budget_in_team: membership?.litellm_budget_table?.max_budget ?? null,
          tpm_limit: membership?.litellm_budget_table?.tpm_limit ?? null,
          rpm_limit: membership?.litellm_budget_table?.rpm_limit ?? null,
          budget_duration: membership?.litellm_budget_table?.budget_duration || null,
          allowed_models: membership?.litellm_budget_table?.allowed_models || [],
        };
        setSelectedEditMember(enhancedMember);
        setIsEditMemberModalVisible(true);
      }}
      onDelete={handleMemberDelete}
      onAddMember={() => setIsAddMemberModalVisible(true)}
      roleColumnTitle={t("Team Role")}
      roleTooltip={t("This role applies only to this team and is independent from the user's proxy-level role.")}
      extraColumns={hasMemberBudgets ? extraColumns : []}
      showDeleteForMember={() =>
        isProxyAdmin || (canEditTeam && !isUserTeamAdmin) || (isUserTeamAdmin && !disableTeamAdminDeleteTeamUser)
      }
    />
  );
}
