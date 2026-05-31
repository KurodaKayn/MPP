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
  const { initialized, login, realLogin, register, loginMethods, loginWithToken, session } =
    useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();
  const nextPath = useMemo(
    () => resolveNextPath(searchParams.get("next")),
    [searchParams],
  );
  
  const [mode, setMode] = useState<"login" | "register" | "mock" | "token">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (initialized && session) {
      router.replace(nextPath);
    }
  }, [initialized, nextPath, router, session]);

  // Set default mode based on available methods
  useEffect(() => {
    if (initialized) {
      if (loginMethods.mock && !username) {
         setMode("mock");
      }
    }
  }, [initialized, loginMethods.mock]);

  const handleAuthSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedUsername = username.trim();
    const normalizedPassword = password.trim();

    if (!normalizedUsername || !normalizedPassword) {
      toast.error("请输入用户名和密码");
      return;
    }

    setSubmitting(true);
    try {
      if (mode === "register") {
        await register(normalizedUsername, normalizedPassword);
        toast.success("注册成功");
      } else {
        await realLogin(normalizedUsername, normalizedPassword);
        toast.success("登录成功");
      }
      router.replace(nextPath);
    } catch (error) {
      toast.error(mode === "register" ? "注册失败" : "登录失败", {
        description: error instanceof Error ? error.message : "请求失败，请稍后再试。",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleMockLoginSubmit = async (event: FormEvent<HTMLFormElement>) => {
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
  };
}
