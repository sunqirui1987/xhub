import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Checkbox } from "@/components/ui/checkbox";
import { userUpdateUserCall, type UserInfoV2Response } from "@/components/networking";
import { normalizeAccountRole, type IamRole } from "@/utils/iamRoles";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";

/** 管理员账户编辑表单。参数 user 为已加载账户、accessToken 为会话、onSaved/onCancel 为保存/取消回调；返回表单。
 * 供用户详情调用，仅提交后台支持的字段；预算留空共享唯一团队余额；分钟上限留空继承上级，失败保留输入，提交期间禁止重复操作。
 */
export default function UserAccountEditor({ user, accessToken, onSaved, onCancel }: {
  user: UserInfoV2Response & { blocked?: boolean };
  accessToken: string;
  onSaved: () => Promise<void>;
  onCancel: () => void;
}) {
  const [email, setEmail] = useState(user.user_email ?? "");
  const [alias, setAlias] = useState(user.user_alias ?? "");
  const [role, setRole] = useState(normalizeAccountRole(user.user_role));
  const [budget, setBudget] = useState(user.max_budget == null ? "" : String(user.max_budget));
  const [rpm, setRpm] = useState(user.rpm_limit == null ? "" : String(user.rpm_limit));
  const [tpm, setTpm] = useState(user.tpm_limit == null ? "" : String(user.tpm_limit));
  const [blocked, setBlocked] = useState(user.blocked ?? false);
  const [saving, setSaving] = useState(false);
  const roles = [{ value: "admin", label: t("Platform administrator") }, { value: "user", label: t("Regular user") }];

  /** 接收浏览器提交事件，保存增量账户字段后刷新详情；返回异步完成状态，错误通过提示展示且保留编辑器。 */
  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    if (saving) return;
    setSaving(true);
    try {
      await userUpdateUserCall(accessToken, { user_id: user.user_id, user_email: email.trim(), user_alias: alias,
        user_role: role, max_budget: budget === "" ? null : Number(budget), rpm_limit: rpm === "" ? null : Number(rpm), tpm_limit: tpm === "" ? null : Number(tpm), blocked }, null);
      await onSaved();
      toast.success(t("User updated successfully"));
    } catch (error) {
      toast.fromError(error);
    } finally {
      setSaving(false);
    }
  };

  return <form onSubmit={save}>
    <fieldset disabled={saving}>
      <FieldGroup>
        <Field><FieldLabel htmlFor="edit-user-email">{t("pages.users.email")}</FieldLabel>
          <Input id="edit-user-email" type="email" required value={email} onChange={(event) => setEmail(event.target.value)} /></Field>
        <Field><FieldLabel htmlFor="edit-user-alias">{t("pages.users.alias")}</FieldLabel>
          <Input id="edit-user-alias" value={alias} onChange={(event) => setAlias(event.target.value)} /></Field>
        <Field><FieldLabel htmlFor="edit-user-role">{t("pages.users.role")}</FieldLabel>
          <Select items={roles} value={role} onValueChange={(value) => value && setRole(value as IamRole)} disabled={saving}>
            <SelectTrigger id="edit-user-role" aria-label={t("pages.users.role")}><SelectValue /></SelectTrigger>
            <SelectContent>{roles.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectContent>
          </Select></Field>
        <Field><FieldLabel htmlFor="edit-user-budget">{t("pages.users.maxBudget")}</FieldLabel>
          <Input id="edit-user-budget" type="number" min="0" step="0.01" placeholder={t("quotaGuide.blank")} value={budget} onChange={(event) => setBudget(event.target.value)} /></Field>
        <p className="text-sm text-muted-foreground">{t("quotaGuide.rates")}</p>
        <Field><FieldLabel htmlFor="edit-user-rpm">{t("Requests per minute Limit (RPM)")}</FieldLabel>
          <Input id="edit-user-rpm" type="number" min="0" max="2147483647" step="1" placeholder={t("quotaGuide.rateBlank")} value={rpm} onChange={(event) => setRpm(event.target.value)} /></Field>
        <Field><FieldLabel htmlFor="edit-user-tpm">{t("Tokens per minute Limit (TPM)")}</FieldLabel>
          <Input id="edit-user-tpm" type="number" min="0" max="2147483647" step="1" placeholder={t("quotaGuide.rateBlank")} value={tpm} onChange={(event) => setTpm(event.target.value)} /></Field>
        <Field orientation="horizontal"><Checkbox id="edit-user-blocked" checked={blocked} onCheckedChange={(value) => setBlocked(value === true)} />
          <FieldLabel htmlFor="edit-user-blocked">{t("Blocked")}</FieldLabel></Field>
      </FieldGroup>
      <div className="mt-6 flex justify-end gap-2">
        <Button type="button" variant="secondary" onClick={onCancel}>{t("common.cancel")}</Button>
        <Button type="submit" aria-busy={saving}>{t("pages.users.saveChanges")}</Button>
      </div>
    </fieldset>
  </form>;
}
