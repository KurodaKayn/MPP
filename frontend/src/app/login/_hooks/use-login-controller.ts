"use client";

import { type FormEvent, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useAuth } from "@/components/auth/auth-provider";

export function resolveNextPath(value: string | null) {
  if (!value || !value.startsWith("/") || value.startsWith("//")) {
    return "/dashboard";
  }

  return value;
}

export function useLoginController() {
  const { initialized, login, loginMethods, loginWithToken, session } =
    useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextPath = useMemo(
    () => resolveNextPath(searchParams.get("next")),
    [searchParams],
  );
  const [username, setUsername] = useState("kuroda_kayn");
  const [accessToken, setAccessToken] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (initialized && session) {
      router.replace(nextPath);
    }
  }, [initialized, nextPath, router, session]);

  const handleLoginSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedUsername = username.trim();

    if (!normalizedUsername) {
      toast.error("请输入用户名");
      return;
    }

    setSubmitting(true);
    try {
      await login(normalizedUsername);
      toast.success("登录成功");
      router.replace(nextPath);
    } catch (error) {
      toast.error("登录失败", {
        description:
          error instanceof Error ? error.message : "请求失败，请稍后再试。",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleTokenLoginSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedToken = accessToken.trim();

    if (!normalizedToken) {
      toast.error("请输入访问令牌");
      return;
    }

    setSubmitting(true);
    try {
      await loginWithToken(normalizedToken);
      router.replace(nextPath);
    } catch (error) {
      toast.error("登录失败", {
        description:
          error instanceof Error ? error.message : "请检查访问令牌是否有效。",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return {
    accessToken,
    handleLoginSubmit,
    handleTokenLoginSubmit,
    initialized,
    loginMethods,
    setAccessToken,
    setUsername,
    submitting,
    username,
  };
}
