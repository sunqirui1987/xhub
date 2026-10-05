import { useQueryClient } from "@tanstack/react-query";
import { FieldGroup } from "@/components/ui/field";
import { FormField } from "@/components/shared/form/FormField";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { CircleHelp, UserPlus } from "lucide-react";
import React, { useState } from "react";
import { useForm } from "react-hook-form";
import TeamDropdown from "./common_components/team_dropdown";
import { toast } from "@/lib/toast";
import { teamMemberAddCall, userCreateCall } from "./networking";
import { t } from "@/i18n";
import { iamRoles } from "@/utils/iamRoles";

interface CreateuserProps {
  userID: string;
  accessToken: string;
  possibleUIRoles: null | Record<string, Record<string, string>>;
  onUserCreated?: (userId: string) => void;
  isEmbedded?: boolean;
}

interface CreateUserFormValues {
  user_email: string;
  user_alias: string;
  password: string;
  user_role: string;
  team_id: string | null;
  team_role: string;
}

const EMPTY_FORM: CreateUserFormValues = {
  user_email: "",
  user_alias: "",
  password: "",
  user_role: iamRoles.RoleUser,
  team_id: null,
  team_role: "user",
};

const accountRoles = () => [
  {
    value: iamRoles.RoleUser,
    label: t("Regular user"),
    description: t(
      "A regular user can call models only through teams they belong to. Team administration is chosen per team, not here.",
    ),
  },
  {
    value: iamRoles.RoleAdmin,
    label: t("Platform administrator"),
    description: t(
      "A platform administrator manages organizations, teams, users, and budgets. They still join a team before they can call models.",
    ),
  },
];

const teamRoles = () => [
  { value: "user", label: t("Team member") },
  { value: "admin", label: t("Team admin") },
];

const labelWithHint = (label: string, hint: string): React.ReactNode => (
  <>
    {label}
    <Tooltip>
      <TooltipTrigger render={<CircleHelp className="size-3.5 shrink-0 cursor-help text-muted-foreground" />} />
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  </>
);

const createdUserId = (response: { data?: { user_id?: string }; user_id?: string } | null | undefined): string =>
  response?.data?.user_id || response?.user_id || "";

export const CreateUserButton: React.FC<CreateuserProps> = ({
  accessToken,
  onUserCreated,
  isEmbedded = false,
}) => {
  const queryClient = useQueryClient();
  const form = useForm<CreateUserFormValues>({ defaultValues: EMPTY_FORM });
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const teamId = form.watch("team_id");
  const roles = accountRoles();
  const membershipRoles = teamRoles();

  const close = () => {
    setOpen(false);
    form.reset(EMPTY_FORM);
  };

  const handleCreate = async (values: CreateUserFormValues) => {
    const email = values.user_email.trim();
    const password = values.password;
    if (!email) {
      toast.fromError(t("pages.users.userEmail"));
      return;
    }
    if (password.length < 8) {
      toast.fromError(t("At least 8 characters. They sign in with this email and password. There is no invitation email."));
      return;
    }
    setSaving(true);
    try {
      const response = await userCreateCall(accessToken, null, {
        user_email: email,
        user_alias: values.user_alias.trim(),
        password,
        user_role: values.user_role || iamRoles.RoleUser,
        auto_create_key: false,
      });
      const userId = createdUserId(response);
      if (values.team_id) {
        try {
          await teamMemberAddCall(accessToken, values.team_id, {
            user_email: email,
            user_id: userId || null,
            role: values.team_role === "admin" ? "admin" : "user",
          });
        } catch (error) {
          toast.fromError(error instanceof Error ? error.message : t("Failed to add team member"));
        }
      }
      await queryClient.invalidateQueries({ queryKey: ["userList"] });
      toast.success(t("Account created. They sign in with this email and the initial password."));
      if (onUserCreated && userId) onUserCreated(userId);
      form.reset(EMPTY_FORM);
      if (!isEmbedded) setOpen(false);
    } catch (error) {
      toast.fromError(error instanceof Error ? error.message : t("Error creating the user"));
    } finally {
      setSaving(false);
    }
  };

  const fields = (
    <FieldGroup>
      <FormField control={form.control} name="user_email" label={t("pages.users.userEmail")}>
        {({ ref, value, ...control }) => (
          <Input {...control} ref={ref} type="email" autoComplete="off" value={value ?? ""} />
        )}
      </FormField>
      <FormField control={form.control} name="user_alias" label={t("Display name")}>
        {({ ref, value, ...control }) => <Input {...control} ref={ref} value={value ?? ""} />}
      </FormField>
      <FormField
        control={form.control}
        name="password"
        label={t("Initial password")}
        description={t("At least 8 characters. They sign in with this email and password. There is no invitation email.")}
      >
        {({ ref, value, ...control }) => (
          <Input {...control} ref={ref} type="password" autoComplete="new-password" value={value ?? ""} />
        )}
      </FormField>
      <FormField
        control={form.control}
        name="user_role"
        label={labelWithHint(
          t("pages.users.role"),
          t("A regular user can call models only through teams they belong to. Team administration is chosen per team, not here."),
        )}
      >
        {({ id, value, onChange }) => (
          <Select items={roles} value={value || iamRoles.RoleUser} onValueChange={(next) => onChange(next ?? iamRoles.RoleUser)}>
            <SelectTrigger id={id} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {roles.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  <span className="font-medium">{option.label}</span>
                  <span className="ml-2 text-xs text-muted-foreground">{option.description}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </FormField>
      <FormField
        control={form.control}
        name="team_id"
        label={t("Add to a team")}
        description={t("Optional. Leave empty and add them from the team page later.")}
      >
        {({ id, value, onChange }) => <TeamDropdown id={id} value={value} onChange={onChange} />}
      </FormField>
      {teamId ? (
        <FormField control={form.control} name="team_role" label={t("Role in this team")}>
          {({ id, value, onChange }) => (
            <Select items={membershipRoles} value={value || "user"} onValueChange={(next) => onChange(next ?? "user")}>
              <SelectTrigger id={id} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {membershipRoles.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </FormField>
      ) : null}
    </FieldGroup>
  );

  const submit = (
    <div className="mt-4 text-right">
      <Button type="submit" disabled={saving}>
        <UserPlus />
        {t("pages.users.invite")}
      </Button>
    </div>
  );

  if (isEmbedded) {
    return (
      <TooltipProvider>
        <form onSubmit={form.handleSubmit(handleCreate)}>
          <p className="mb-4 text-sm text-muted-foreground">
            {t("Create an account. Choose whether they administer the whole platform, then optionally put them on one team.")}
          </p>
          {fields}
          {submit}
        </form>
      </TooltipProvider>
    );
  }

  return (
    <>
      <Button type="button" onClick={() => setOpen(true)}>
        + {t("pages.users.invite")}
      </Button>
      <Dialog open={open} onOpenChange={(next) => !next && close()}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[640px]">
          <DialogHeader>
            <DialogTitle>{t("pages.users.invite")}</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            {t("Create an account. Choose whether they administer the whole platform, then optionally put them on one team.")}
          </p>
          <TooltipProvider>
            <form onSubmit={form.handleSubmit(handleCreate)}>
              {fields}
              {submit}
            </form>
          </TooltipProvider>
        </DialogContent>
      </Dialog>
    </>
  );
};
