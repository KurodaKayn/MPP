"use client";

import { Suspense } from "react";
import { ArrowRight, KeyRound, Loader2, LogIn, UserPlus } from "lucide-react";
import Image from "next/image";
import { useTranslation, useAppLocale } from "@/lib/i18n/client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLoginController } from "./_hooks/use-login-controller";

function LoginContent() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const { t: tHome } = useTranslation(locale, "home");
  const {
    accessToken,
    handleLoginSubmit,
    handleRegisterSubmit,
    handleTokenLoginSubmit,
    initialized,
    loginMethods,
    password,
    registerPassword,
    registerPasswordConfirm,
    registerUsername,
    setAccessToken,
    setPassword,
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
        <section className="relative hidden overflow-hidden border-r border-[#d9d4c8] bg-[#1f2520] p-10 text-[#f6f4ee] lg:flex lg:flex-col lg:justify-between">
          <div className="absolute inset-0 opacity-20 [background-image:linear-gradient(#f6f4ee_1px,transparent_1px),linear-gradient(90deg,#f6f4ee_1px,transparent_1px)] [background-size:42px_42px]" />
          <div className="relative flex items-center gap-3">
            <Image src="/icons/mpp.svg" alt="MPP" width={34} height={34} />
            <div className="leading-tight">
              <div className="font-semibold">MPP</div>
            </div>
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

        <section className="flex min-h-svh items-center justify-center px-5 py-10">
          <div className="w-full max-w-sm">
            <div className="mb-8 flex items-center gap-3 lg:hidden">
              <Image src="/icons/mpp.svg" alt="MPP" width={32} height={32} />
              <div className="leading-tight">
                <div className="font-semibold">MPP</div>
              </div>
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
                <TabsList className="grid h-10 w-full grid-cols-2 rounded-md border border-[#d9d4c8] bg-white/60 p-1">
                  <TabsTrigger value="login" className="rounded-[4px]">
                    <LogIn className="h-4 w-4" />
                    {t("login.signInTab")}
                  </TabsTrigger>
                  <TabsTrigger value="register" className="rounded-[4px]">
                    <UserPlus className="h-4 w-4" />
                    {t("login.registerTab")}
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="login">
                  <form className="space-y-5" onSubmit={handleLoginSubmit}>
                    <div className="space-y-2">
                      <Label htmlFor="username">{t("login.username")}</Label>
                      <Input
                        id="username"
                        autoComplete="username"
                        className="h-10 border-[#cfc8ba] bg-white/70"
                        value={username}
                        onChange={(event) => setUsername(event.target.value)}
                        placeholder={t("login.usernamePlaceholder")}
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="password">{t("login.password")}</Label>
                      <Input
                        id="password"
                        type="password"
                        autoComplete="current-password"
                        className="h-10 border-[#cfc8ba] bg-white/70"
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        placeholder={t("login.passwordPlaceholder")}
                      />
                    </div>

                    <Button
                      type="submit"
                      className="h-10 w-full bg-[#1f2520] text-[#f6f4ee] hover:bg-[#303830]"
                      disabled={submitting || !initialized}
                    >
                      {submitting ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <LogIn className="h-4 w-4" />
                      )}
                      {t("login.submit")}
                      <ArrowRight className="ml-auto h-4 w-4" />
                    </Button>
                  </form>
                </TabsContent>

                <TabsContent value="register">
                  <form className="space-y-5" onSubmit={handleRegisterSubmit}>
                    <div className="space-y-2">
                      <Label htmlFor="register-username">
                        {t("login.username")}
                      </Label>
                      <Input
                        id="register-username"
                        autoComplete="username"
                        className="h-10 border-[#cfc8ba] bg-white/70"
                        value={registerUsername}
                        onChange={(event) =>
                          setRegisterUsername(event.target.value)
                        }
                        placeholder={t("login.usernamePlaceholder")}
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="register-password">
                        {t("login.password")}
                      </Label>
                      <Input
                        id="register-password"
                        type="password"
                        autoComplete="new-password"
                        className="h-10 border-[#cfc8ba] bg-white/70"
                        value={registerPassword}
                        onChange={(event) =>
                          setRegisterPassword(event.target.value)
                        }
                        placeholder={t("login.registerPasswordPlaceholder")}
                      />
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="register-password-confirm">
                        {t("login.confirmPassword")}
                      </Label>
                      <Input
                        id="register-password-confirm"
                        type="password"
                        autoComplete="new-password"
                        className="h-10 border-[#cfc8ba] bg-white/70"
                        value={registerPasswordConfirm}
                        onChange={(event) =>
                          setRegisterPasswordConfirm(event.target.value)
                        }
                        placeholder={t("login.confirmPasswordPlaceholder")}
                      />
                    </div>

                    <Button
                      type="submit"
                      className="h-10 w-full bg-[#1f2520] text-[#f6f4ee] hover:bg-[#303830]"
                      disabled={submitting || !initialized}
                    >
                      {submitting ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <UserPlus className="h-4 w-4" />
                      )}
                      {t("login.registerSubmit")}
                      <ArrowRight className="ml-auto h-4 w-4" />
                    </Button>

                    <p className="text-xs leading-5 text-[#667064]">
                      <KeyRound className="mr-1 inline h-3.5 w-3.5" />
                      {t("login.passwordHint")}
                    </p>
                  </form>
                </TabsContent>
              </Tabs>
            ) : (
              <form className="space-y-5" onSubmit={handleTokenLoginSubmit}>
                <div className="space-y-2">
                  <Label htmlFor="access-token">{t("login.accessToken")}</Label>
                  <Input
                    id="access-token"
                    type="password"
                    autoComplete="off"
                    className="h-10 border-[#cfc8ba] bg-white/70"
                    value={accessToken}
                    onChange={(event) => setAccessToken(event.target.value)}
                  />
                </div>

                <Button
                  type="submit"
                  className="h-10 w-full bg-[#1f2520] text-[#f6f4ee] hover:bg-[#303830]"
                  disabled={submitting || !initialized}
                >
                  {submitting ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <LogIn className="h-4 w-4" />
                  )}
                  {t("login.submit")}
                  <ArrowRight className="ml-auto h-4 w-4" />
                </Button>
              </form>
            )}
          </div>
        </section>
      </div>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginContent />
    </Suspense>
  );
}
