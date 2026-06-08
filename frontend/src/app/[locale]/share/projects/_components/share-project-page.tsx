"use client";

import { ArrowRight, Loader2, RefreshCw, UsersRound } from "lucide-react";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth/auth-provider";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { acceptProjectShareLink } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

type AcceptState = "idle" | "accepting" | "accepted" | "error";

type ShareProjectPageProps = {
  token?: string;
};

const selectedWorkspaceStorageKey = "mpp.dashboard.selectedWorkspaceId";

function syncWorkspaceContextForSharedProject(workspaceId?: string | null) {
  try {
    if (workspaceId) {
      window.localStorage.setItem(selectedWorkspaceStorageKey, workspaceId);
      return;
    }
    window.localStorage.removeItem(selectedWorkspaceStorageKey);
  } catch {
    // Ignore private-mode or disabled storage failures; the redirect still works.
  }
}

export function ShareProjectPage({ token = "" }: ShareProjectPageProps) {
  const { initialized, session } = useAuth();
  const pathname = usePathname();
  const router = useRouter();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const [state, setState] = useState<AcceptState>("idle");
  const [errorMessage, setErrorMessage] = useState("");
  const [retryKey, setRetryKey] = useState(0);
  const acceptedTokenRef = useRef("");

  useEffect(() => {
    if (!initialized) {
      return;
    }
    if (!session) {
      const nextPath = token
        ? `${pathname}?token=${encodeURIComponent(token)}`
        : pathname;
      router.replace(`/${locale}/login?next=${encodeURIComponent(nextPath)}`);
      return;
    }
    if (!token || acceptedTokenRef.current === token) {
      return;
    }

    acceptedTokenRef.current = token;
    setState("accepting");
    setErrorMessage("");

    async function accept() {
      try {
        const response = await acceptProjectShareLink(token);
        setState("accepted");
        syncWorkspaceContextForSharedProject(response.project.workspace_id);
        toast.success(t("shareProject.accepted"));
        router.replace(
          `/${locale}/dashboard/content?projectId=${encodeURIComponent(response.project.id)}`,
        );
      } catch (error) {
        acceptedTokenRef.current = "";
        setState("error");
        setErrorMessage(
          error instanceof Error ? error.message : t("shareProject.retryLater"),
        );
      }
    }

    void accept();
  }, [initialized, locale, pathname, retryKey, router, session, t, token]);

  const isBusy = !initialized || state === "accepting" || state === "accepted";

  return (
    <main className="flex min-h-svh items-center justify-center bg-muted/20 p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <Badge variant="outline" className="mb-2 w-fit gap-1">
            <UsersRound className="size-3" />
            {t("shareProject.badge")}
          </Badge>
          <CardTitle>{t("shareProject.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {state === "error"
              ? t("shareProject.errorDescription")
              : t("shareProject.description")}
          </p>
          {state === "error" ? (
            <div className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              {errorMessage || t("shareProject.retryLater")}
            </div>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              onClick={() => {
                acceptedTokenRef.current = "";
                setState("idle");
                setRetryKey((current) => current + 1);
              }}
              disabled={isBusy || !token}
            >
              {isBusy ? (
                <Loader2 className="size-4 animate-spin" />
              ) : state === "error" ? (
                <RefreshCw className="size-4" />
              ) : (
                <ArrowRight className="size-4" />
              )}
              {state === "error"
                ? t("shareProject.retry")
                : t("shareProject.continue")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}
