"use client";

import { ConfigType, useProxyConfig } from "@/app/(dashboard)/hooks/proxyConfig/useProxyConfig";
import { useStoreRequestInSpendLogs } from "@/app/(dashboard)/hooks/storeRequestInSpendLogs/useStoreRequestInSpendLogs";
import { parseErrorMessage } from "@/components/shared/errorUtils";
import { FormField } from "@/components/shared/form/FormField";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { FieldGroup } from "@/components/ui/field";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { t } from "@/i18n";
import { toast } from "@/lib/toast";
import { CircleHelp } from "lucide-react";
import React, { useEffect } from "react";
import { useForm } from "react-hook-form";

const STORE_PROMPTS_FIELD_NAME = "store_prompts_in_spend_logs";

interface LoggingSettingsFormValues {
  store_prompts_in_spend_logs: boolean;
}

const LoggingSettings: React.FC = () => {
  const { mutate, isPending } = useStoreRequestInSpendLogs();
  const { data: proxyConfigData, isLoading } = useProxyConfig(ConfigType.GENERAL_SETTINGS);
  const form = useForm<LoggingSettingsFormValues>({
    defaultValues: { store_prompts_in_spend_logs: false },
  });

  const stored = proxyConfigData?.find((field) => field.field_name === STORE_PROMPTS_FIELD_NAME)?.field_value;

  useEffect(() => {
    form.reset({ store_prompts_in_spend_logs: Boolean(stored) });
  }, [form, stored]);

  const onSubmit = (values: LoggingSettingsFormValues) => {
    mutate(
      { store_prompts_in_spend_logs: values.store_prompts_in_spend_logs },
      {
        onSuccess: () => toast.success(t("Spend logs settings updated successfully")),
        onError: (error) => toast.fromError(t("Failed to save spend logs settings: ") + parseErrorMessage(error)),
      },
    );
  };

  return (
    <Card>
      <CardHeader className="border-b">
        <CardTitle>{t("Logging Settings")}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="mb-6 text-muted-foreground">
          {t("Proxy-wide settings that control how request and response data are written to spend logs.")}
        </p>
        {isLoading ? (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-4 w-2/5" />
            <Skeleton className="h-4 w-full" />
          </div>
        ) : (
          <TooltipProvider>
            <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
              <FieldGroup>
                <FormField
                  control={form.control}
                  name={STORE_PROMPTS_FIELD_NAME}
                  label={
                    <>
                      {t("Store Prompts in Spend Logs")}
                      <Tooltip>
                        <TooltipTrigger
                          render={<CircleHelp className="size-3.5 shrink-0 cursor-help text-muted-foreground" />}
                        />
                        <TooltipContent>
                          {t("When enabled, prompts will be stored in spend logs for tracking and analysis purposes.")}
                        </TooltipContent>
                      </Tooltip>
                    </>
                  }
                >
                  {({ id, value, onChange, onBlur }) => (
                    <Switch id={id} checked={Boolean(value)} onCheckedChange={onChange} onBlur={onBlur} className="w-fit" />
                  )}
                </FormField>
              </FieldGroup>
              <Button type="submit" className="mt-6" disabled={isPending}>
                {isPending && <UiLoadingSpinner role="img" aria-label={t("loading")} className="size-4" />}
                {isPending ? t("Saving...") : t("Save Settings")}
              </Button>
            </form>
          </TooltipProvider>
        )}
      </CardContent>
    </Card>
  );
};

export default LoggingSettings;
