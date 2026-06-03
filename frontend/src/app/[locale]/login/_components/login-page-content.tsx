"use client";

import type { ComponentType } from "react";

import { ArrowRight, KeyRound, Loader2, LogIn, UserPlus } from "lucide-react";
import Image from "next/image";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

import { useLoginController } from "../_hooks/use-login-controller";

type LoginFormFieldProps = {
  autoComplete?: string;
  id: string;
  label: string;
  onValueChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  value: string;
};

function BrandMark() {
  return (
    <div className="flex items-center gap-3">
      <Image src="/icons/mpp.svg" alt="MPP" width={32} height={32} />
      <div className="leading-tight">
        <div className="font-semibold">MPP</div>
      </div>
    </div>
  );
}

function LoginSidePanel({ t, tHome }: { t: any; tHome: any }) {
  return (
    <section className="relative hidden overflow-hidden border-r border-[#d9d4c8] bg-[#1f2520] p-10 text-[#f6f4ee] lg:flex lg:flex-col lg:justify-between">
      <div className="absolute inset-0 opacity-20 [background-image:linear-gradient(#f6f4ee_1px,transparent_1px),linear-gradient(90deg,#f6f4ee_1px,transparent_1px)] [background-size:42px_42px]" />
      <div className="relative">
        <BrandMark />
      </div>

      <div className="relative max-w-2xl">
        <div className="mb-5 inline-flex rounded-md border border-[#f0c75e]/50 px-2 py-1 text-xs text-[#f0c75e]">
          Creator Console
        </div>
        <h1 className="max-w-xl text-5xl font-semibold leading-tight tracking-normal">
          {t("login.sideTitle")}
        </h1>
      </div>

      <div className="relative grid grid-cols-3 gap-3 text-sm text-[#d8d2c2]">
        {[
          tHome("workflow.step1.title"),
          tHome("workflow.step2.title"),
          tHome("workflow.step4.title"),
        ].map((item) => (
          <div key={item} className="border-t border-[#f0c75e]/40 pt-3">
            {item}
          </div>
        ))}
      </div>
    </section>
  );
}

function LoginFormField({
  autoComplete,
  id,
  label,
  onValueChange,
  placeholder,
  type,
  value,
}: LoginFormFieldProps) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        autoComplete={autoComplete}
        className="h-10 border-[#cfc8ba] bg-white/70"
        value={value}
        onChange={(event) => onValueChange(event.target.value)}
        placeholder={placeholder}
      />
    </div>
  );
}

function LoginSubmitButton({
  initialized,
  label,
  submitting,
  icon: Icon,
}: {
  initialized: boolean;
  label: string;
  submitting: boolean;
  icon: ComponentType<{ className?: string }>;
}) {
  return (
    <Button
      type="submit"
      className="h-10 w-full bg-[#1f2520] text-[#f6f4ee] hover:bg-[#303830]"
      disabled={submitting || !initialized}
    >
      {submitting ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : (
        <Icon className="h-4 w-4" />
      )}
      {label}
      <ArrowRight className="ml-auto h-4 w-4" />
    </Button>
  );
}

export function LoginPageContent() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const { t: tHome } = useTranslation(locale, "home");
  const {
    accessToken,
    forgotPasswordCode,
    forgotPasswordEmail,
    handleLoginSubmit,
    handleRegisterSubmit,
    handleResetPasswordSubmit,
    handleSendCode,
    handleTokenLoginSubmit,
    initialized,
    loginMethods,
    newPassword,
    password,
    registerCode,
    registerEmail,
    registerPassword,
    registerPasswordConfirm,
    registerUsername,
    sendingCode,
    setAccessToken,
    setForgotPasswordCode,
    setForgotPasswordEmail,
    setNewPassword,
    setPassword,
    setRegisterCode,
    setRegisterEmail,
    setRegisterPassword,
    setRegisterPasswordConfirm,
    setRegisterUsername,
    setUsername,
    submitting,
    username,
  } = useLoginController();

  return (
    <main className="min-h-svh bg-[#f6f4ee] text-[#1f2520]">
      <div className="grid min-h-svh lg:grid-cols-[minmax(0,1fr)_460px]">
        <LoginSidePanel t={t} tHome={tHome} />

        <section className="flex min-h-svh items-center justify-center px-5 py-10">
          <div className="w-full max-w-sm">
            <div className="mb-8 lg:hidden">
              <BrandMark />
            </div>

            <div className="mb-7">
              <h2 className="text-2xl font-semibold tracking-normal">
                {t("login.title")}
              </h2>
              <p className="mt-2 text-sm text-[#667064]">
                {loginMethods.mock ? t("login.mockDesc") : t("login.tokenDesc")}
              </p>
            </div>

            {loginMethods.mock ? (
              <Tabs defaultValue="login" className="gap-6">
                <TabsList className="grid h-10 w-full grid-cols-3 rounded-md border border-[#d9d4c8] bg-white/60 p-1">
                  <TabsTrigger value="login" className="rounded-[4px] text-xs">
                    <LogIn className="h-3.5 w-3.5" />
                    {t("login.signInTab")}
                  </TabsTrigger>
                  <TabsTrigger
                    value="register"
                    className="rounded-[4px] text-xs"
                  >
                    <UserPlus className="h-3.5 w-3.5" />
                    {t("login.registerTab")}
                  </TabsTrigger>
                  <TabsTrigger value="forgot" className="rounded-[4px] text-xs">
                    <KeyRound className="h-3.5 w-3.5" />
                    {t("login.forgotPasswordTab", { defaultValue: "Forgot" })}
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="login">
                  <form className="space-y-5" onSubmit={handleLoginSubmit}>
                    <LoginFormField
                      id="username"
                      autoComplete="username"
                      label={t("login.username")}
                      value={username}
                      onValueChange={setUsername}
                      placeholder={t("login.usernamePlaceholder")}
                    />
                    <LoginFormField
                      id="password"
                      type="password"
                      autoComplete="current-password"
                      label={t("login.password")}
                      value={password}
                      onValueChange={setPassword}
                      placeholder={t("login.passwordPlaceholder")}
                    />
                    <LoginSubmitButton
                      initialized={initialized}
                      submitting={submitting}
                      label={t("login.submit")}
                      icon={LogIn}
                    />
                  </form>
                </TabsContent>

                <TabsContent value="register">
                  <form className="space-y-5" onSubmit={handleRegisterSubmit}>
                    <LoginFormField
                      id="register-username"
                      autoComplete="username"
                      label={t("login.username")}
                      value={registerUsername}
                      onValueChange={setRegisterUsername}
                      placeholder={t("login.usernamePlaceholder")}
                    />
                    <div className="space-y-2">
                      <Label htmlFor="register-email">{t("login.email")}</Label>
                      <div className="flex gap-2">
                        <Input
                          id="register-email"
                          type="email"
                          autoComplete="email"
                          className="h-10 border-[#cfc8ba] bg-white/70"
                          value={registerEmail}
                          onChange={(e) => setRegisterEmail(e.target.value)}
                          placeholder={t("login.emailPlaceholder")}
                        />
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="h-10 border-[#cfc8ba]"
                          disabled={sendingCode || !registerEmail}
                          onClick={() =>
                            handleSendCode(registerEmail, "register")
                          }
                        >
                          {sendingCode ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            t("login.getCode")
                          )}
                        </Button>
                      </div>
                    </div>
                    <LoginFormField
                      id="register-code"
                      label={t("login.code")}
                      value={registerCode}
                      onValueChange={setRegisterCode}
                      placeholder={t("login.codePlaceholder")}
                    />
                    <LoginFormField
                      id="register-password"
                      type="password"
                      autoComplete="new-password"
                      label={t("login.password")}
                      value={registerPassword}
                      onValueChange={setRegisterPassword}
                      placeholder={t("login.registerPasswordPlaceholder")}
                    />
                    <LoginFormField
                      id="register-password-confirm"
                      type="password"
                      autoComplete="new-password"
                      label={t("login.confirmPassword")}
                      value={registerPasswordConfirm}
                      onValueChange={setRegisterPasswordConfirm}
                      placeholder={t("login.confirmPasswordPlaceholder")}
                    />
                    <LoginSubmitButton
                      initialized={initialized}
                      submitting={submitting}
                      label={t("login.registerSubmit")}
                      icon={UserPlus}
                    />
                  </form>
                </TabsContent>

                <TabsContent value="forgot">
                  <form
                    className="space-y-5"
                    onSubmit={handleResetPasswordSubmit}
                  >
                    <div className="space-y-2">
                      <Label htmlFor="forgot-email">{t("login.email")}</Label>
                      <div className="flex gap-2">
                        <Input
                          id="forgot-email"
                          type="email"
                          autoComplete="email"
                          className="h-10 border-[#cfc8ba] bg-white/70"
                          value={forgotPasswordEmail}
                          onChange={(e) =>
                            setForgotPasswordEmail(e.target.value)
                          }
                          placeholder={t("login.emailPlaceholder")}
                        />
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="h-10 border-[#cfc8ba]"
                          disabled={sendingCode || !forgotPasswordEmail}
                          onClick={() =>
                            handleSendCode(forgotPasswordEmail, "forgot_password")
                          }
                        >
                          {sendingCode ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            t("login.getCode")
                          )}
                        </Button>
                      </div>
                    </div>
                    <LoginFormField
                      id="forgot-code"
                      label={t("login.code")}
                      value={forgotPasswordCode}
                      onValueChange={setForgotPasswordCode}
                      placeholder={t("login.codePlaceholder")}
                    />
                    <LoginFormField
                      id="new-password"
                      type="password"
                      autoComplete="new-password"
                      label={t("login.newPassword", {
                        defaultValue: "New Password",
                      })}
                      value={newPassword}
                      onValueChange={setNewPassword}
                      placeholder={t("login.newPasswordPlaceholder")}
                    />
                    <LoginSubmitButton
                      initialized={initialized}
                      submitting={submitting}
                      label={t("login.resetSubmit", { defaultValue: "Reset" })}
                      icon={KeyRound}
                    />
                  </form>
                </TabsContent>
              </Tabs>
            ) : (
              <form className="space-y-5" onSubmit={handleTokenLoginSubmit}>
                <LoginFormField
                  id="access-token"
                  type="password"
                  autoComplete="off"
                  label={t("login.accessToken")}
                  value={accessToken}
                  onValueChange={setAccessToken}
                />
                <LoginSubmitButton
                  initialized={initialized}
                  submitting={submitting}
                  label={t("login.submit")}
                  icon={LogIn}
                />
              </form>
            )}
          </div>
        </section>
      </div>
    </main>
  );
}
