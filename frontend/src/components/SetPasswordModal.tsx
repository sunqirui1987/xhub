import React from "react";
import { useForm } from "react-hook-form";
import { FieldGroup } from "@/components/ui/field";
import { FormField } from "@/components/shared/form/FormField";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { toast } from "@/lib/toast";
import { t } from "@/i18n";
import { userSetPasswordCall } from "@/components/networking";

/**
 * The minimum the server enforces when it hashes a password. It is repeated
 * here so a short one is refused before a round trip, not to be the source of
 * truth: iam.hashPassword is, and a client that skipped this check would still
 * get a 400.
 */
export const MIN_PASSWORD_LENGTH = 8;

interface SetPasswordFormValues {
  password: string;
  confirm: string;
}

const EMPTY_FORM: SetPasswordFormValues = { password: "", confirm: "" };

interface SetPasswordModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accessToken: string | null;
  /** The account whose password is being replaced. */
  user: { user_id: string; user_email?: string | null; user_alias?: string | null } | null;
  onSuccess?: () => void;
}

/**
 * SetPasswordModal replaces an account's password directly.
 *
 * It replaces the old reset-link modal. That modal called /invitation/new,
 * built a URL to /ui/onboarding, and asked the administrator to send it on. The
 * link could never work, because the onboarding page reads a signed token that
 * no handler mints and the claim endpoint drops the password field on the
 * floor. There is nothing to send: the administrator types the new password
 * here and tells the person what it is.
 */
export default function SetPasswordModal({ open, onOpenChange, accessToken, user, onSuccess }: SetPasswordModalProps) {
  const form = useForm<SetPasswordFormValues>({ defaultValues: EMPTY_FORM });

  const close = () => {
    form.reset(EMPTY_FORM);
    onOpenChange(false);
  };

  const handleSubmit = async (values: SetPasswordFormValues) => {
    if (!accessToken || !user?.user_id) {
      toast.fromError(t("Access token not found"));
      return;
    }
    if (values.password.length < MIN_PASSWORD_LENGTH) {
      toast.fromError(t("The password must be at least 8 characters."));
      return;
    }
    if (values.password !== values.confirm) {
      toast.fromError(t("The two passwords do not match."));
      return;
    }
    try {
      await userSetPasswordCall(accessToken, user.user_id, values.password);
      toast.success(t("Password updated. Their existing sessions have been signed out."));
      form.reset(EMPTY_FORM);
      onOpenChange(false);
      onSuccess?.();
    } catch (error) {
      toast.fromError(error instanceof Error ? error.message : t("Failed to update the password"));
    }
  };

  // The dialog names the account it is about to change, so the administrator
  // can see they opened the right row. Any of the three identifies it.
  const identities = [user?.user_alias, user?.user_email, user?.user_id];
  const subject = identities.find((value) => Boolean(value)) ?? "";

  return (
    <Dialog open={open} onOpenChange={(next) => !next && close()}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>{t("Set a new password")}</DialogTitle>
        </DialogHeader>
        <form onSubmit={form.handleSubmit(handleSubmit)}>
          <p className="text-sm text-muted-foreground">
            {t("You are setting a new password for")}{" "}
            <span className="font-medium text-foreground">{subject}</span>.{" "}
            {t("Tell them the new password directly. Their open sessions end immediately.")}
          </p>
          <FieldGroup>
            <FormField
              control={form.control}
              name="password"
              label={t("New password")}
              description={t("At least 8 characters.")}
            >
              {({ ref, value, ...control }) => (
                <Input
                  {...control}
                  ref={ref}
                  value={value ?? ""}
                  type="password"
                  autoComplete="new-password"
                />
              )}
            </FormField>
            <FormField control={form.control} name="confirm" label={t("Confirm the new password")}>
              {({ ref, value, ...control }) => (
                <Input
                  {...control}
                  ref={ref}
                  value={value ?? ""}
                  type="password"
                  autoComplete="new-password"
                />
              )}
            </FormField>
          </FieldGroup>
          <div className="mt-4 flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={close}>
              {t("common.cancel")}
            </Button>
            <Button type="submit">{t("Save the new password")}</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
