"use client";

import { useLogin } from "@/app/(dashboard)/hooks/login/useLogin";
import { useUIConfig } from "@/app/(dashboard)/hooks/uiConfig/useUIConfig";
import LoadingScreen from "@/components/common_components/LoadingScreen";
import { exchangeLoginCode, switchToWorkerUrl } from "@/components/networking";
import { Alert, AlertDescription, AlertTitle } from "@/components/shared/Alert";
import { PasswordInput } from "@/components/shared/PasswordInput";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { FormField } from "@/components/shared/form/FormField";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { UiLoadingSpinner } from "@/components/ui/ui-loading-spinner";
import { useZodForm } from "@/lib/forms/useZodForm";
import { clearTokenCookies, getCookieFromDocument } from "@/utils/cookieUtils";
import { isJwtExpired } from "@/utils/jwtUtils";
import { consumeReturnUrl } from "@/utils/returnUrlUtils";
import { Activity, Boxes, CircleAlert, ShieldCheck, TriangleAlert } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useId, useMemo, useState, type ReactNode } from "react";
import { z } from "zod/v4";
import { useWorker } from "@/hooks/useWorker";
import { t, useI18n } from "@/i18n";
import LanguageSwitcher from "@/components/LanguageSwitcher";

type LoginFormValues = { username: string; password: string };

function LoginLogo() {
  return (
    <div data-testid="login-logo" className="flex shrink-0 flex-col items-start gap-4">
      <svg role="img" aria-label={t("site.logoAlt")} width="56" height="56" viewBox="0 0 84 84">
        <rect width="84" height="84" rx="18" fill="#3c8dbc" />
        <text x="42" y="54" textAnchor="middle" fontSize="36" fontWeight="700" fill="#ffffff">
          X
        </text>
      </svg>
      <div className="text-3xl font-semibold tracking-tight text-primary-foreground">{t("site.product")}</div>
    </div>
  );
}

function LoginStage({ children }: { children: ReactNode }) {
  return (
    <div data-testid="login-stage" className="relative flex h-dvh min-h-dvh w-full overflow-y-auto bg-canvas">
      <div className="absolute top-5 right-5 z-10">
        <LanguageSwitcher />
      </div>
      <div className="grid min-h-full w-full lg:grid-cols-[minmax(0,1fr)_minmax(28rem,42rem)]">
        <section className="flex min-h-[34rem] flex-col justify-center bg-primary px-8 py-16 text-primary-foreground sm:px-14 lg:min-h-dvh lg:px-[clamp(3.5rem,8vw,9rem)]">
          <div className="mx-auto w-full max-w-xl">
            <LoginLogo />
            <div className="mt-12 max-w-lg">
              <p className="text-sm font-semibold uppercase tracking-[0.18em] text-primary-foreground/65">
                {t("login.brandEyebrow")}
              </p>
              <h2 className="mt-5 text-4xl font-semibold tracking-tight sm:text-5xl">{t("login.brandTitle")}</h2>
              <p className="mt-6 max-w-md text-lg leading-8 text-primary-foreground/75">
                {t("login.brandDescription")}
              </p>
            </div>
            <div className="mt-12 grid max-w-sm gap-5 border-t border-primary-foreground/20 pt-8 sm:grid-cols-3 lg:grid-cols-1">
              {[
                { icon: Boxes, label: t("login.featureModels") },
                { icon: ShieldCheck, label: t("login.featureAccess") },
                { icon: Activity, label: t("login.featureObservability") },
              ].map(({ icon: Icon, label }) => (
                <div key={label} className="flex items-center gap-3 text-sm font-medium text-primary-foreground/85">
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary-foreground/12">
                    <Icon className="size-4" />
                  </span>
                  <span>{label}</span>
                </div>
              ))}
            </div>
          </div>
        </section>
        <section className="flex min-h-[38rem] items-center justify-center bg-card px-6 py-16 sm:px-12 lg:min-h-dvh lg:px-16">
          <div
            data-testid="login-card"
            className="w-full max-w-md rounded-2xl border border-border bg-card px-7 py-9 text-card-foreground shadow-[0_20px_60px_rgba(15,23,42,0.12)] sm:px-10"
          >
            {children}
          </div>
        </section>
      </div>
    </div>
  );
}

function LoginPageContent() {
  const [isLoading, setIsLoading] = useState(true);
  const { data: uiConfig, isLoading: isConfigLoading } = useUIConfig();
  const loginMutation = useLogin();
  const router = useRouter();
  const { workers, selectWorker } = useWorker();
  const [selectedWorkerId, setSelectedWorkerId] = useState<string | null>(null);
  const workerFieldId = useId();
  const { locale } = useI18n();
  const loginSchema = useMemo(
    () =>
      z.object({
        username: z.string().min(1, t("login.usernameRequired")),
        password: z.string().min(1, t("login.passwordRequired")),
      }),
    [locale],
  );
  const form = useZodForm(loginSchema, { defaultValues: { username: "", password: "" } });

  // Pre-select worker from URL param (e.g. /ui/login?worker=team-b)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const workerParam = params.get("worker");
    if (workerParam) {
      setSelectedWorkerId(workerParam);
    }
  }, []);

  useEffect(() => {
    if (isConfigLoading) {
      return;
    }

    // Check if admin UI is disabled
    if (uiConfig && uiConfig.admin_ui_disabled) {
      setIsLoading(false);
      return;
    }

    // Cross-origin SSO: worker redirected back with a single-use code.
    // Exchange it for the JWT via the worker's /v3/login/exchange endpoint.
    const params = new URLSearchParams(window.location.search);
    const rawSsoCode = params.get("code");
    // Validate the SSO code is a plausible OAuth authorization code (alphanumeric
    // plus common URL-safe chars) so that arbitrary user input cannot trigger the
    // exchange endpoint.
    const ssoCode = rawSsoCode && /^[a-zA-Z0-9._~+/=-]+$/.test(rawSsoCode) ? rawSsoCode : null;
    if (ssoCode) {
      const rawWorkerUrl = localStorage.getItem("litellm_worker_url");
      // Validate the stored worker URL: only allow http(s) URLs.
      const workerUrl = rawWorkerUrl && /^https?:\/\/.+/.test(rawWorkerUrl) ? rawWorkerUrl : null;
      exchangeLoginCode(ssoCode, workerUrl).then(() => {
        params.delete("code");
        const cleanSearch = params.toString();
        window.history.replaceState(null, "", window.location.pathname + (cleanSearch ? `?${cleanSearch}` : ""));
        router.replace("/ui/?login=success");
      });
      return;
    }

    // If switching workers on a control plane, clear the old token and show login
    const switchingWorker = params.has("worker");
    if (switchingWorker && uiConfig?.is_control_plane) {
      clearTokenCookies();
      setIsLoading(false);
      return;
    }

    const rawToken = getCookieFromDocument("token");
    if (rawToken && !isJwtExpired(rawToken)) {
      // User already logged in - redirect to return URL or default
      const returnUrl = consumeReturnUrl();
      if (returnUrl) {
        router.replace(returnUrl);
      } else {
        router.replace("/ui");
      }
      return;
    }

    setIsLoading(false);
  }, [isConfigLoading, router, uiConfig]);

  const handleSubmit = ({ username, password }: LoginFormValues) => {
    // If a worker is selected, point proxyBaseUrl at it before login
    const selectedWorker = workers.find((w) => w.worker_id === selectedWorkerId);
    if (selectedWorker) {
      switchToWorkerUrl(selectedWorker.url);
    }

    loginMutation.mutate(
      { username, password, useV3: !!selectedWorker },
      {
        onSuccess: (data) => {
          // Update the worker context with the selected worker
          if (selectedWorker) {
            selectWorker(selectedWorker.worker_id);
            // Stay on the CP's UI — proxyBaseUrl already points at the worker
            router.push("/ui/?login=success");
          } else {
            // Normal (non-control-plane) login — follow the server's redirect
            const returnUrl = consumeReturnUrl();
            if (returnUrl) {
              router.push(returnUrl);
            } else {
              router.push(data.redirect_url);
            }
          }
        },
        onError: () => {
          // Reset proxyBaseUrl on login failure
          if (selectedWorker) {
            switchToWorkerUrl(null);
          }
        },
      },
    );
  };

  const error = loginMutation.error instanceof Error ? loginMutation.error.message : null;
  const isLoginLoading = loginMutation.isPending;

  if (isConfigLoading || isLoading) {
    return (
      <LoginStage>
        <div className="flex w-full flex-col gap-6 text-center">
          <h1 className="text-3xl font-semibold text-foreground">{t("login.title")}</h1>
          <p className="text-lg text-muted-foreground">{t("login.subtitle")}</p>
          <LoadingScreen />
        </div>
      </LoginStage>
    );
  }

  // Show disabled message if admin UI is disabled
  if (uiConfig && uiConfig.admin_ui_disabled) {
    return (
      <LoginStage>
        <div className="flex w-full flex-col gap-6">
          <div className="text-center">
            <h1 className="text-3xl font-semibold text-foreground">{t("login.title")}</h1>
          </div>

          <Alert variant="warning">
            <TriangleAlert />
            <AlertTitle>{t("login.adminDisabled")}</AlertTitle>
            <AlertDescription>
              <p className="text-sm">{t("login.adminDisabledBody")}</p>
              <p className="mt-2 text-sm">
                <code className="rounded-sm bg-muted px-1 py-0.5 text-xs">DISABLE_ADMIN_UI=False</code>
              </p>
            </AlertDescription>
          </Alert>
        </div>
      </LoginStage>
    );
  }

  return (
    <LoginStage>
      <TooltipProvider>
        <div className="flex w-full flex-col gap-4">
          <div className="text-center">
            <h1 className="text-2xl font-semibold text-foreground">{t("login.title")}</h1>
            <p className="mt-1 text-base text-muted-foreground">{t("login.subtitle")}</p>
          </div>

          {error && (
            <Alert variant="error">
              <CircleAlert />
              <AlertTitle>{error}</AlertTitle>
            </Alert>
          )}

          <form onSubmit={form.handleSubmit(handleSubmit)}>
            <FieldGroup className="gap-4">
              {uiConfig?.is_control_plane && workers.length > 0 && (
                <Field>
                  <FieldLabel htmlFor={workerFieldId}>{t("login.worker")}</FieldLabel>
                  <Select
                    items={workers.map((worker) => ({ label: worker.name, value: worker.worker_id }))}
                    value={selectedWorkerId}
                    onValueChange={(value: string | null) => setSelectedWorkerId(value)}
                  >
                    <SelectTrigger id={workerFieldId} className="h-10 w-full">
                      <SelectValue placeholder={t("login.workerPlaceholder")} />
                    </SelectTrigger>
                    <SelectContent>
                      {workers.map((worker) => (
                        <SelectItem key={worker.worker_id} value={worker.worker_id}>
                          {worker.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
              )}

              <FormField control={form.control} name="username" label={t("login.username")}>
                {({ ref, ...field }) => (
                  <Input
                    {...field}
                    ref={ref}
                    placeholder={t("login.usernamePlaceholder")}
                    autoComplete="username"
                    disabled={isLoginLoading}
                    className="h-11 rounded-sm border-input bg-background text-base"
                  />
                )}
              </FormField>

              <FormField control={form.control} name="password" label={t("login.password")}>
                {({ ref, ...field }) => (
                  <PasswordInput
                    {...field}
                    ref={ref}
                    placeholder={t("login.passwordPlaceholder")}
                    autoComplete="current-password"
                    disabled={isLoginLoading}
                    groupClassName="h-11"
                  />
                )}
              </FormField>

              <Button
                type="submit"
                size="lg"
                disabled={isLoginLoading}
                className="h-11 w-full rounded-sm bg-primary text-base font-semibold text-primary-foreground hover:bg-primary/80"
              >
                {isLoginLoading && <UiLoadingSpinner className="size-4" role="img" aria-label={t("loading")} />}
                {isLoginLoading ? t("login.submitting") : t("login.submit")}
              </Button>

            </FieldGroup>
          </form>
        </div>
      </TooltipProvider>
    </LoginStage>
  );
}

export default function LoginPage() {
  return <LoginPageContent />;
}
