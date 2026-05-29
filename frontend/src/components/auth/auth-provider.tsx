"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  clearAuthSession,
  getStoredAuthSession,
  loginWithUsername,
  subscribeAuthChanged,
  type AuthSession,
} from "@/lib/auth/client";

type AuthContextValue = {
  initialized: boolean;
  session: AuthSession | null;
  login: (username: string) => Promise<AuthSession>;
  logout: () => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [initialized, setInitialized] = useState(false);
  const [session, setSession] = useState<AuthSession | null>(null);

  const refreshSession = useCallback(() => {
    setSession(getStoredAuthSession());
    setInitialized(true);
  }, []);

  useEffect(() => {
    refreshSession();
    return subscribeAuthChanged(refreshSession);
  }, [refreshSession]);

  const login = useCallback(async (username: string) => {
    const nextSession = await loginWithUsername(username);
    setSession(nextSession);
    setInitialized(true);
    return nextSession;
  }, []);

  const logout = useCallback(() => {
    clearAuthSession();
    setSession(null);
    setInitialized(true);
  }, []);

  const value = useMemo(
    () => ({ initialized, login, logout, session }),
    [initialized, login, logout, session],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider");
  }

  return context;
}
