import GuardrailGarden from "./guardrail_garden";
import { parseAsString, useQueryState } from "nuqs";
import React, { useState, useEffect, useCallback } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ChevronDown, Code, Plus, ShieldCheck, Search, ArrowRight } from "lucide-react";
import { getGuardrailsList, deleteGuardrailCall } from "@/components/networking";
import { Input } from "@/components/ui/input";
import { buttonVariants } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/cva.config";
import LocalGuardrailModal from "./LocalGuardrailEditor";
import GuardrailTable from "./guardrail_table";
import { isAdminRole } from "@/utils/roles";
import GuardrailInfoView from "./guardrail_info";
import GuardrailTestPlayground from "./GuardrailTestPlayground";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";
import { Guardrail } from "@/components/guardrails/types";
import DeleteResourceModal from "@/components/common_components/DeleteResourceModal";
import { formatGuardrailMode, getGuardrailLogoAndName } from "./guardrail_info_helpers";
import { CustomCodeModal } from "./custom_code";

import { TeamGuardrailsTab } from "./TeamGuardrailsTab";

interface GuardrailsPanelProps {
  accessToken: string | null;
  userRole?: string;
}

interface GuardrailsResponse {
  guardrails: Guardrail[];
}

/**
 * 用途：组织花园、规则、调试与团队提交入口，按管理权限控制配置操作。
 * 参数：accessToken：管理令牌；userRole：显示管理入口的角色。
 * 返回：React 面板；实际授权仍由后端校验。
 * 调用：护栏页面。
 * 测试：子组件集成测试；主面板尚无专门集成测试。
 */
const GuardrailsPanel: React.FC<GuardrailsPanelProps> = ({ accessToken, userRole }) => {
  const [guardrailsList, setGuardrailsList] = useState<Guardrail[]>([]);
  const [isAddModalVisible, setIsAddModalVisible] = useState(false);
  const [isCustomCodeModalVisible, setIsCustomCodeModalVisible] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [search, setSearch] = useState("");
  const [isDeleting, setIsDeleting] = useState(false);
  const [guardrailToDelete, setGuardrailToDelete] = useState<Guardrail | null>(null);
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [selectedGuardrailId, setSelectedGuardrailId] = useQueryState(
    "guardrail",
    parseAsString.withOptions({ history: "push" }),
  );
  const isAdmin = userRole ? isAdminRole(userRole) : false;

  /**
   * 用途：获取服务器规则目录，兼容缺失数组并保留加载失败提示。
   * 参数：无；读取 accessToken。
   * 返回：Promise<void>；更新规则、加载状态和错误。
   * 调用：首次加载、创建成功、退出详情和删除后的刷新。
   * 测试：静态检查；数组兼容逻辑无独立测试。
   */
  const fetchGuardrails = useCallback(async () => {
    if (!accessToken) {
      return;
    }

    setIsLoading(true);
    try {
      const response: GuardrailsResponse = await getGuardrailsList(accessToken);
      setGuardrailsList(Array.isArray(response.guardrails) ? response.guardrails : []);
      setLoadError("");
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : t("护栏加载失败"));
    } finally {
      setIsLoading(false);
    }
  }, [accessToken]);

  useEffect(() => {
    fetchGuardrails();
  }, [fetchGuardrails]);

  /**
   * 用途：清除 URL 中的护栏选择并重新读取规则列表。
   * 参数：无；读取组件当前状态。
   * 返回：void；异步更新导航和列表。
   * 调用：详情返回及打开创建弹窗。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const closeGuardrailDetail = () => {
    void setSelectedGuardrailId(null, { history: "replace" });
    void fetchGuardrails();
  };

  /**
   * 用途：打开本地关键词/正则编辑器，退出当前详情。
   * 参数：无；读取组件当前状态。
   * 返回：void；更新弹窗状态。
   * 调用：新增本地护栏按钮。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleAddGuardrail = () => {
    if (selectedGuardrailId) {
      closeGuardrailDetail();
    }
    setIsAddModalVisible(true);
  };

  /**
   * 用途：打开 XGo 编辑器，退出当前详情。
   * 参数：无；读取组件当前状态。
   * 返回：void；更新弹窗状态。
   * 调用：创建 XGo 按钮。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleAddCustomCodeGuardrail = () => {
    if (selectedGuardrailId) {
      closeGuardrailDetail();
    }
    setIsCustomCodeModalVisible(true);
  };

  /**
   * 用途：关闭本地编辑器。
   * 参数：无；读取组件当前状态。
   * 返回：void；更新弹窗状态。
   * 调用：本地编辑器 onClose。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleCloseModal = () => {
    setIsAddModalVisible(false);
  };

  /**
   * 用途：关闭 XGo 编辑器。
   * 参数：无；读取组件当前状态。
   * 返回：void；更新弹窗状态。
   * 调用：XGo 编辑器 onClose。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleCloseCustomCodeModal = () => {
    setIsCustomCodeModalVisible(false);
  };

  /**
   * 用途：保存成功后重新获取服务器规则。
   * 参数：无；读取组件当前状态。
   * 返回：void；启动异步列表刷新。
   * 调用：创建和编辑 onSuccess。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleSuccess = () => {
    fetchGuardrails();
  };

  /**
   * 用途：选择数据库规则并展示删除确认；配置文件规则提示在 YAML 删除。
   * 参数：guardrailId：规则 ID；guardrailName：兼容列表回调签名，实际名称来自规则。
   * 返回：void；更新确认状态或显示错误提示。
   * 调用：规则行删除按钮。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleDeleteClick = (guardrailId: string, guardrailName: string) => {
    const guardrail = guardrailsList.find((g) => g.guardrail_id === guardrailId) || null;
    if (guardrail?.guardrail_definition_location === "config") {
      toast.fromError(t("该护栏来自 YAML，请在配置文件中删除。"));
      return;
    }
    setGuardrailToDelete(guardrail);
    setIsDeleteModalOpen(true);
  };

  /**
   * 用途：删除已选数据库规则，刷新列表并释放忙碌状态。
   * 参数：无；读取待删除规则和令牌。
   * 返回：Promise<void>；失败显示错误，finally 清除确认状态。
   * 调用：删除确认按钮。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleDeleteConfirm = async () => {
    if (!guardrailToDelete || !accessToken) return;

    setIsDeleting(true);
    try {
      await deleteGuardrailCall(accessToken, guardrailToDelete.guardrail_id);
      toast.success(t('Guardrail "{value0}" deleted successfully', { value0: guardrailToDelete.guardrail_name }));
      await fetchGuardrails();
    } catch (error) {
      console.error("Error deleting guardrail:", error);
      toast.fromError(t("Failed to delete guardrail"));
    } finally {
      setIsDeleting(false);
      setIsDeleteModalOpen(false);
      setGuardrailToDelete(null);
    }
  };

  /**
   * 用途：取消删除并清除待删除规则。
   * 参数：无；读取组件当前状态。
   * 返回：void；恢复列表状态。
   * 调用：删除确认弹窗取消按钮。
   * 测试：现有组件测试间接覆盖；本辅助函数暂无独立单测。
   */
  const handleDeleteCancel = () => {
    setIsDeleteModalOpen(false);
    setGuardrailToDelete(null);
  };

  const providerDisplayName =
    guardrailToDelete && guardrailToDelete.litellm_params
      ? getGuardrailLogoAndName(guardrailToDelete.litellm_params.guardrail).displayName
      : undefined;

  return (
    <div className="w-full min-w-0 space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <span className="flex size-11 shrink-0 items-center justify-center rounded-xl border bg-card text-primary">
            <ShieldCheck className="size-5" />
          </span>
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">{t("护栏管理")}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{t("统一管理请求检查、内容脱敏与自定义审核。")}</p>
          </div>
        </div>
        <span className="rounded-full border bg-card px-3 py-1.5 text-xs text-muted-foreground">
          {t("调用前检查")} <ArrowRight className="mx-1 inline size-3" /> {t("模型请求")}
        </span>
      </div>
      <Tabs defaultValue={isAdmin ? "guardrails" : "submitted"}>
        <TabsList variant="line" className="max-w-full overflow-x-auto border-b">
          {isAdmin && (
            <>
              <TabsTrigger value="garden" className="flex-none">
                {t("pages.guardrails.garden")}
              </TabsTrigger>
              <TabsTrigger value="guardrails" className="flex-none">
                {t("pages.guardrails.title")}
              </TabsTrigger>
              <TabsTrigger value="playground" className="flex-none" disabled={!accessToken}>
                {t("pages.guardrails.playground")}
              </TabsTrigger>
            </>
          )}
          <TabsTrigger value="submitted" className="flex-none">
            {t("pages.guardrails.submitted")}
          </TabsTrigger>
        </TabsList>

        {isAdmin && (
          <>
            <TabsContent value="garden" keepMounted>
              <GuardrailGarden
                accessToken={accessToken}
                onGuardrailCreated={fetchGuardrails}
                onCreateLocal={handleAddGuardrail}
                onCreateXGo={handleAddCustomCodeGuardrail}
              />
            </TabsContent>

            <TabsContent value="guardrails" keepMounted>
              <div className="mb-6 grid gap-3 sm:grid-cols-3">
                {[
                  [t("全部护栏"), guardrailsList.length],
                  [t("默认启用"), guardrailsList.filter((g) => g.litellm_params.default_on).length],
                  [t("按需执行"), guardrailsList.filter((g) => !g.litellm_params.default_on).length],
                ].map(([label, value]) => (
                  <div key={label} className="flex items-center justify-between rounded-xl border bg-card px-5 py-4">
                    <span className="text-sm text-muted-foreground">{label}</span>
                    <span className="text-2xl font-semibold tabular-nums">{isLoading ? "—" : value}</span>
                  </div>
                ))}
              </div>
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <div className="relative w-full sm:max-w-sm">
                  <Search aria-hidden="true" className="absolute left-3 top-2.5 size-4 text-muted-foreground" />
                  <Input
                    aria-label={t("搜索护栏")}
                    placeholder={t("搜索名称或实现类型")}
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    className="bg-card pl-9"
                  />
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger disabled={!accessToken} className={cn(buttonVariants({ variant: "default" }))}>
                    <Plus />
                    {t("pages.guardrails.create")}
                    <ChevronDown />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" className="w-56">
                    <DropdownMenuItem onClick={handleAddGuardrail}>
                      <Plus />
                      {t("关键词 / 正则护栏")}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={handleAddCustomCodeGuardrail}>
                      <Code />
                      {t("XGo 自定义脚本护栏")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>

              {loadError && (
                <p role="alert" className="text-destructive">
                  {loadError}
                </p>
              )}
              {selectedGuardrailId ? (
                <GuardrailInfoView
                  guardrailId={selectedGuardrailId}
                  onClose={closeGuardrailDetail}
                  accessToken={accessToken}
                  isAdmin={isAdmin}
                />
              ) : (
                <GuardrailTable
                  guardrailsList={guardrailsList.filter((g) =>
                    [
                      g.guardrail_name,
                      g.litellm_params.guardrail,
                      getGuardrailLogoAndName(g.litellm_params.guardrail).displayName,
                    ]
                      .join(" ")
                      .toLowerCase()
                      .includes(search.trim().toLowerCase()),
                  )}
                  isLoading={isLoading}
                  onDeleteClick={handleDeleteClick}
                  onGuardrailClick={(id) => void setSelectedGuardrailId(id)}
                />
              )}

              <LocalGuardrailModal
                visible={isAddModalVisible}
                onClose={handleCloseModal}
                accessToken={accessToken}
                onSuccess={handleSuccess}
              />

              <CustomCodeModal
                visible={isCustomCodeModalVisible}
                onClose={handleCloseCustomCodeModal}
                accessToken={accessToken}
                onSuccess={handleSuccess}
              />

              <DeleteResourceModal
                isOpen={isDeleteModalOpen}
                title={t("Delete Guardrail")}
                message={t("Are you sure you want to delete guardrail: {value0}? This action cannot be undone.", {
                  value0: guardrailToDelete?.guardrail_name,
                })}
                resourceInformationTitle="Guardrail Information"
                resourceInformation={[
                  { label: t("Name"), value: guardrailToDelete?.guardrail_name },
                  { label: "ID", value: guardrailToDelete?.guardrail_id, code: true },
                  { label: t("Provider"), value: providerDisplayName },
                  { label: t("Mode"), value: formatGuardrailMode(guardrailToDelete?.litellm_params.mode) },
                  {
                    label: t("Default On"),
                    value: guardrailToDelete?.litellm_params.default_on ? t("Yes") : t("No"),
                  },
                ]}
                onCancel={handleDeleteCancel}
                onOk={handleDeleteConfirm}
                confirmLoading={isDeleting}
              />
            </TabsContent>

            <TabsContent value="playground" keepMounted>
              <div className="rounded-xl border bg-card p-4 sm:p-6">
                <GuardrailTestPlayground
                  guardrailsList={guardrailsList}
                  isLoading={isLoading}
                  accessToken={accessToken}
                  onClose={() => {}}
                />
              </div>
            </TabsContent>
          </>
        )}

        <TabsContent value="submitted" keepMounted>
          <TeamGuardrailsTab accessToken={accessToken} />
        </TabsContent>
      </Tabs>
    </div>
  );
};

export default GuardrailsPanel;
