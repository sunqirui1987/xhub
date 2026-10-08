"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";
import { modelPatchUpdateCall } from "@/components/networking";
import { Switch } from "@/components/ui/switch";
import { isProxyAdminRole } from "@/utils/roles";
import { toast } from "@/lib/toast";
import type { EditorModel } from "./modelEditorPricing";

export default function ModelStatusToggle({ model }: { model: EditorModel }) {
  const { accessToken, userRole, isViewOnly } = useAuthorized();
  const client = useQueryClient();
  const [saving, setSaving] = useState(false);
  const enabled = model.model_info.disabled !== true;
  const editable = !isViewOnly && isProxyAdminRole(userRole ?? "") && model.model_info.db_model === true;
  const change = async (checked: boolean) => {
    if (!editable || !accessToken) return;
    setSaving(true);
    try {
      await modelPatchUpdateCall(accessToken, { model_info: { disabled: !checked } }, String(model.model_info.id));
      await client.invalidateQueries({ queryKey: ["models"] });
      await client.invalidateQueries({ queryKey: ["allProxyModels"] });
      await client.invalidateQueries({ queryKey: ["userModels"] });
      await client.invalidateQueries({ queryKey: ["modelHub"] });
      await client.invalidateQueries({ queryKey: ["selectedTeamModels"] });
      toast.success(checked ? "模型已启用" : "模型已停用");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "更新模型状态失败");
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="flex items-center gap-2 whitespace-nowrap">
      <Switch
        checked={enabled}
        disabled={!editable || saving}
        onCheckedChange={change}
        aria-label={model.model_name + " 启用模型"}
      />
      <span className="text-xs text-muted-foreground">{enabled ? "已启用" : "已停用"}</span>
    </div>
  );
}
