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
  const { initialized, login, loginMethods, loginWithToken, session } =
    useAuth();
  const router = useRouter();
  const searchParams = useSearchParams();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
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

  const handleMockLoginSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedUsername = username.trim();

    if (!normalizedUsername) {
      toast.error(t("login.usernameRequired"));
      return;
    }

    setSubmitting(true);
    try {
      await login(normalizedUsername);
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

  return {
    accessToken,
    handleMockLoginSubmit,
    handleTokenLoginSubmit,
    initialized,
    loginMethods,
    setAccessToken,
    setUsername,
    submitting,
    username,
  };
}
