"use client";

import { Loader2 } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect } from "react";
import { useAuth } from "./auth-provider";

function AuthGuardFallback() {
  return (
    <div className="flex min-h-svh items-center justify-center bg-background text-muted-foreground">
      <Loader2 className="h-5 w-5 animate-spin" />
    </div>
  );
}

function AuthGuardContent({ children }: { children: React.ReactNode }) {
  const { initialized, session } = useAuth();
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();

  useEffect(() => {
    if (!initialized || session) {
      return;
    }

    const query = searchParams.toString();
    const next = query ? `${pathname}?${query}` : pathname;
    router.replace(`/login?next=${encodeURIComponent(next)}`);
  }, [initialized, pathname, router, searchParams, session]);

  if (!initialized || !session) {
    return <AuthGuardFallback />;
  }

  return children;
}

export function AuthGuard({ children }: { children: React.ReactNode }) {
  return (
    <Suspense fallback={<AuthGuardFallback />}>
      <AuthGuardContent>{children}</AuthGuardContent>
    </Suspense>
  );
}
