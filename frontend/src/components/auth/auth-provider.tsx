"use client";

import { useEffect } from "react";
import { subscribeAuthChanged } from "@/lib/auth/client";
import { useAuthStore } from "./auth-store";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const refresh = useAuthStore((state) => state.refresh);

  useEffect(() => {
    void refresh();
    return subscribeAuthChanged(() => {
      void refresh();
    });
  }, [refresh]);

  return children;
}

export function useAuth() {
  return useAuthStore();
}
