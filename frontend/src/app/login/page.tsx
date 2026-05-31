"use client";

import { Suspense } from "react";
import { ArrowRight, Loader2, LogIn, UserPlus } from "lucide-react";
import Image from "next/image";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLoginController } from "./_hooks/use-login-controller";

function LoginContent() {
  const {
    accessToken,
    handleAuthSubmit,
    handleMockLoginSubmit,
    handleTokenLoginSubmit,
    initialized,
    loginMethods,
    mode,
    setMode,
    password,
    setPassword,
    setAccessToken,
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
            <Image
              src="/icons/mpp.svg"
              alt="multi-platform poster"
              width={34}
              height={34}
            />
            <div className="leading-tight">
              <div className="font-semibold">multi-platform poster</div>
            </div>
          </div>

          <div className="relative max-w-2xl">
            <div className="mb-5 inline-flex rounded-md border border-[#f0c75e]/50 px-2 py-1 text-xs text-[#f0c75e]">
              Creator Console
            </div>
            <h1 className="max-w-xl text-5xl font-semibold leading-tight tracking-normal">
              从一篇内容开始，管理所有发布渠道。
            </h1>
          </div>

          <div className="relative grid grid-cols-3 gap-3 text-sm text-[#d8d2c2]">
            {["草稿", "适配", "分发"].map((item) => (
              <div key={item} className="border-t border-[#f0c75e]/40 pt-3">
                {item}
              </div>
            ))}
          </div>
        </section>

        <section className="flex min-h-svh items-center justify-center px-5 py-10">
          <div className="w-full max-w-sm">
            <div className="mb-8 flex items-center gap-3 lg:hidden">
              <Image
                src="/icons/mpp.svg"
                alt="multi-platform poster"
                width={32}
                height={32}
              />
              <div className="leading-tight">
                <div className="font-semibold">multi-platform poster</div>
              </div>
            </div>

            <div className="mb-7">
              <h2 className="text-2xl font-semibold tracking-normal">
                {mode === "register" ? "创建账号" : mode === "token" ? "令牌登录" : mode === "mock" ? "开发测试登录" : "登录控制台"}
              </h2>
              <p className="mt-2 text-sm text-[#667064]">
                {mode === "register" 
                  ? "加入 MPP，开启多平台分发之旅。" 
                  : mode === "token" 
                    ? "使用访问令牌进入工作台。" 
                    : mode === "mock"
                      ? "使用开发账号直接进入。"
                      : "欢迎回来，请输入您的凭据。"}
              </p>
            </div>

            {(mode === "login" || mode === "register") ? (
              <form className="space-y-5" onSubmit={handleAuthSubmit}>
                <div className="space-y-2">
                  <Label htmlFor="username">用户名</Label>
                  <Input
                    id="username"
                    autoComplete="username"
                    className="h-10 border-[#cfc8ba] bg-white/70"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="password">密码</Label>
                  <Input
                    id="password"
                    type="password"
                    autoComplete={mode === "login" ? "current-password" : "new-password"}
                    className="h-10 border-[#cfc8ba] bg-white/70"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                  />
                </div>

                <Button
                  type="submit"
                  className="h-10 w-full bg-[#1f2520] text-[#f6f4ee] hover:bg-[#303830]"
                  disabled={submitting || !initialized}
                >
                  {submitting ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : mode === "register" ? (
                    <UserPlus className="h-4 w-4" />
                  ) : (
                    <LogIn className="h-4 w-4" />
                  )}
                  {mode === "register" ? "立即注册" : "登录"}
                  <ArrowRight className="ml-auto h-4 w-4" />
                </Button>

                <div className="pt-2 text-center text-sm">
                   <button 
                    type="button"
                    onClick={() => setMode(mode === "login" ? "register" : "login")}
                    className="text-[#667064] hover:text-[#1f2520] underline underline-offset-4"
                   >
                     {mode === "login" ? "还没有账号？立即注册" : "已有账号？返回登录"}
                   </button>
                </div>
              </form>
            ) : mode === "mock" ? (
              <form className="space-y-5" onSubmit={handleMockLoginSubmit}>
                <div className="space-y-2">
                  <Label htmlFor="username">用户名</Label>
                  <Input
                    id="username"
                    autoComplete="username"
                    className="h-10 border-[#cfc8ba] bg-white/70"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
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
                  进入工作台 (Mock)
                  <ArrowRight className="ml-auto h-4 w-4" />
                </Button>
                
                <div className="pt-2 text-center text-sm">
                   <button 
                    type="button"
                    onClick={() => setMode("login")}
                    className="text-[#667064] hover:text-[#1f2520] underline underline-offset-4"
                   >
                     返回标准登录
                   </button>
                </div>
              </form>
            ) : (
              <form className="space-y-5" onSubmit={handleTokenLoginSubmit}>
                <div className="space-y-2">
                  <Label htmlFor="access-token">访问令牌</Label>
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
                  进入工作台 (Token)
                  <ArrowRight className="ml-auto h-4 w-4" />
                </Button>

                <div className="pt-2 text-center text-sm">
                   <button 
                    type="button"
                    onClick={() => setMode("login")}
                    className="text-[#667064] hover:text-[#1f2520] underline underline-offset-4"
                   >
                     返回标准登录
                   </button>
                </div>
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
