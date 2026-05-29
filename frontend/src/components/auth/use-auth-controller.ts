"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  clearAuthSession,
  clearServerAuthSession,
  getCurrentAuthState,
  loginWithAccessToken,
  loginWithUsername,
  subscribeAuthChanged,
  type AuthLoginMethods,
  type AuthSession,
} from "@/lib/auth/client";

export type AuthContextValue = {
  initialized: boolean;
  loginMethods: AuthLoginMethods;
  session: AuthSession | null;
  login: (username: string) => Promise<AuthSession>;
  loginWithToken: (token: string) => Promise<AuthSession>;
  logout: () => Promise<void>;
};

export function useAuthController(): AuthContextValue {
  const [initialized, setInitialized] = useState(false);
  const [loginMethods, setLoginMethods] = useState<AuthLoginMethods>({
    mock: false,
    token: true,
  });
  const [session, setSession] = useState<AuthSession | null>(null);

  useEffect(() => {
    let active = true;

    const refresh = async () => {
      const authState = await getCurrentAuthState();
      if (!active) {
        return;
      }

      setLoginMethods(authState.loginMethods);
      setSession(authState.session);
      setInitialized(true);
    };

    void refresh();
    const unsubscribe = subscribeAuthChanged(() => {
      void refresh();
    });

    return () => {
      active = false;
      unsubscribe();
    };
  }, []);

  const login = useCallback(async (username: string) => {
    const nextSession = await loginWithUsername(username);
    setSession(nextSession);
    setInitialized(true);
    return nextSession;
  }, []);

  const loginWithToken = useCallback(async (token: string) => {
    const nextSession = await loginWithAccessToken(token);
    setSession(nextSession);
    setInitialized(true);
    return nextSession;
  }, []);

  const logout = useCallback(async () => {
    clearAuthSession({ notify: false });
    await clearServerAuthSession();
    setSession(null);
    setInitialized(true);
  }, []);

  return useMemo(
    () => ({
      initialized,
      login,
      loginMethods,
      loginWithToken,
      logout,
      session,
    }),
    [initialized, login, loginMethods, loginWithToken, logout, session],
  );
}
