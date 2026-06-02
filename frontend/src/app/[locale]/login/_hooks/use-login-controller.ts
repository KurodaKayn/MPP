"use client";

import { type FormEvent, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useAuth } from "@/components/auth/auth-provider";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

export function resolveNextPath(value: string | null) {
  if (!value || !value.startsWith("/") || value.startsWith("//")) {
    return "/dashboard";
  }

  return value;
}

export function useLoginController() {
  const {
    initialized,
    login,
    loginMethods,
    loginWithToken,
    register,
    session,
  } = useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const nextPath = useMemo(
    () => resolveNextPath(searchParams.get("next")),
    [searchParams],
  );
  const [username, setUsername] = useState("kuroda_kayn");
  const [password, setPassword] = useState("");
  const [registerUsername, setRegisterUsername] = useState("");
  const [registerPassword, setRegisterPassword] = useState("");
  const [registerPasswordConfirm, setRegisterPasswordConfirm] = useState("");
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
      toast.error(t("login.usernameRequired"));
      return;
    }

    if (!password) {
      toast.error(t("login.passwordRequired"));
      return;
    }

    setSubmitting(true);
    try {
      await login(normalizedUsername, password);
      toast.success(t("login.success", { defaultValue: "Login successful" }));
      router.replace(nextPath);
    } catch (error) {
      toast.error(t("login.failed"), {
        description:
          error instanceof Error ? error.message : t("login.mockDescError"),
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleTokenLoginSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedToken = accessToken.trim();

    if (!normalizedToken) {
      toast.error(t("login.tokenRequired"));
      return;
    }

    setSubmitting(true);
    try {
      await loginWithToken(normalizedToken);
      router.replace(nextPath);
    } catch (error) {
      toast.error(t("login.failed"), {
        description:
          error instanceof Error ? error.message : t("login.tokenDescError"),
      });
    } finally {
      setSubmitting(false);
    }
  };

  const handleRegisterSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedUsername = registerUsername.trim();

    if (!normalizedUsername) {
      toast.error(t("login.usernameRequired"));
      return;
    }

    if (!registerPassword) {
      toast.error(t("login.passwordRequired"));
      return;
    }

    if (registerPassword !== registerPasswordConfirm) {
      toast.error(t("login.passwordMismatch"));
      return;
    }

    setSubmitting(true);
    try {
      await register(normalizedUsername, registerPassword);
      toast.success(t("login.registerSuccess"));
      router.replace(nextPath);
    } catch (error) {
      toast.error(t("login.registerFailed"), {
        description:
          error instanceof Error ? error.message : t("login.registerDescError"),
      });
    } finally {
      setSubmitting(false);
    }
  };

  return {
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
  };
}
