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
    resetPassword,
    sendCode,
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
  const [registerEmail, setRegisterEmail] = useState("");
  const [registerCode, setRegisterCode] = useState("");
  const [registerPassword, setRegisterPassword] = useState("");
  const [registerPasswordConfirm, setRegisterPasswordConfirm] = useState("");

  const [forgotPasswordEmail, setForgotPasswordEmail] = useState("");
  const [forgotPasswordCode, setForgotPasswordCode] = useState("");
  const [newPassword, setNewPassword] = useState("");

  const [accessToken, setAccessToken] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [sendingCode, setSendingCode] = useState(false);

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

  const handleSendCode = async (email: string, scene: string) => {
    if (!email.trim()) {
      toast.error(t("login.emailRequired"));
      return;
    }

    setSendingCode(true);
    try {
      await sendCode(email, scene);
      toast.success(t("login.codeSent"));
    } catch (error) {
      toast.error(t("login.codeSendFailed"), {
        description: error instanceof Error ? error.message : String(error),
      });
    } finally {
      setSendingCode(false);
    }
  };

  const handleRegisterSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedUsername = registerUsername.trim();
    const normalizedEmail = registerEmail.trim();

    if (!normalizedUsername) {
      toast.error(t("login.usernameRequired"));
      return;
    }

    if (!normalizedEmail) {
      toast.error(t("login.emailRequired"));
      return;
    }

    if (!registerCode) {
      toast.error(t("login.codeRequired"));
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
      await register(
        normalizedUsername,
        normalizedEmail,
        registerCode,
        registerPassword,
      );
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

  const handleResetPasswordSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedEmail = forgotPasswordEmail.trim();

    if (!normalizedEmail) {
      toast.error(t("login.emailRequired"));
      return;
    }

    if (!forgotPasswordCode) {
      toast.error(t("login.codeRequired"));
      return;
    }

    if (!newPassword) {
      toast.error(t("login.passwordRequired"));
      return;
    }

    setSubmitting(true);
    try {
      await resetPassword(normalizedEmail, forgotPasswordCode, newPassword);
      toast.success(t("login.resetSuccess"));
      // Reset flow or redirect to login tab
      setForgotPasswordCode("");
      setNewPassword("");
    } catch (error) {
      toast.error(t("login.resetFailed"), {
        description: error instanceof Error ? error.message : String(error),
      });
    } finally {
      setSubmitting(false);
    }
  };

  return {
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
  };
}
