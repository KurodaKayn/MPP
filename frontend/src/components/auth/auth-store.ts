"use client";

import { create } from "zustand";
import {
  clearAuthSession,
  clearServerAuthSession,
  getCurrentAuthState,
  loginWithAccessToken,
  loginWithUsername,
  loginWithPassword,
  registerUser,
  type AuthLoginMethods,
  type AuthSession,
} from "@/lib/auth/client";

export type AuthStoreState = {
  initialized: boolean;
  loginMethods: AuthLoginMethods;
  session: AuthSession | null;
};

type AuthStoreActions = {
  login: (username: string) => Promise<AuthSession>;
  loginWithToken: (token: string) => Promise<AuthSession>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  reset: () => void;
};

export type AuthStore = AuthStoreState & AuthStoreActions;

const initialState: AuthStoreState = {
  initialized: false,
  loginMethods: {
    mock: false,
    token: true,
  },
  session: null,
};

export const useAuthStore = create<AuthStore>((set) => ({
  ...initialState,
  login: async (username) => {
    const nextSession = await loginWithUsername(username);
    set({ initialized: true, session: nextSession });
    return nextSession;
  },
  loginWithToken: async (token) => {
    const nextSession = await loginWithAccessToken(token);
    set({ initialized: true, session: nextSession });
    return nextSession;
  },
  logout: async () => {
    clearAuthSession({ notify: false });
    await clearServerAuthSession();
    set({ initialized: true, session: null });
  },
  refresh: async () => {
    const authState = await getCurrentAuthState();
    set({
      initialized: true,
      loginMethods: authState.loginMethods,
      session: authState.session,
    });
  },
  reset: () => set(initialState),
}));
