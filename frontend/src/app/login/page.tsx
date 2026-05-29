"use client";

import { FormEvent, Suspense, useEffect, useMemo, useState } from "react";
import { ArrowRight, Loader2, LogIn } from "lucide-react";
import Image from "next/image";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useAuth } from "@/components/auth/auth-provider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function resolveNextPath(value: string | null) {
  if (!value || !value.startsWith("/") || value.startsWith("//")) {
    return "/dashboard";
  }

  return value;
}

function LoginContent() {
  const { initialized, login, session } = useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextPath = useMemo(
    () => resolveNextPath(searchParams.get("next")),
    [searchParams],
  );
  const [username, setUsername] = useState("kuroda_kayn");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (initialized && session) {
      router.replace(nextPath);
    }
  }, [initialized, nextPath, router, session]);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedUsername = username.trim();

    if (!normalizedUsername) {
      toast.error("请输入用户名");
      return;
    }

    setSubmitting(true);
    try {
      await login(normalizedUsername);
      router.replace(nextPath);
    } catch (error) {
      toast.error("登录失败", {
        description:
          error instanceof Error ? error.message : "请检查开发账号是否存在。",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <main className="min-h-svh bg-[#f6f4ee] text-[#1f2520]">
      <div className="grid min-h-svh lg:grid-cols-[minmax(0,1fr)_460px]">
        <section className="relative hidden overflow-hidden border-r border-[#d9d4c8] bg-[#1f2520] p-10 text-[#f6f4ee] lg:flex lg:flex-col lg:justify-between">
          <div className="absolute inset-0 opacity-20 [background-image:linear-gradient(#f6f4ee_1px,transparent_1px),linear-gradient(90deg,#f6f4ee_1px,transparent_1px)] [background-size:42px_42px]" />
          <div className="relative flex items-center gap-3">
            <Image
              src="/icons/mpp.svg"
              alt="multi-plantform poster"
              width={34}
              height={34}
            />
            <div className="leading-tight">
              <div className="font-semibold">multi-plantform poster</div>
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
                alt="multi-plantform poster"
                width={32}
                height={32}
              />
              <div className="leading-tight">
                <div className="font-semibold">multi-plantform poster</div>
              </div>
            </div>

            <div className="mb-7">
              <h2 className="text-2xl font-semibold tracking-normal">
                登录控制台
              </h2>
              <p className="mt-2 text-sm text-[#667064]">
                使用开发账号进入工作台。
              </p>
            </div>

            <form className="space-y-5" onSubmit={handleSubmit}>
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
                进入工作台
                <ArrowRight className="ml-auto h-4 w-4" />
              </Button>
            </form>
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
