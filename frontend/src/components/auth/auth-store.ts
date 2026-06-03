"use client";

import { create } from "zustand";
import {
  clearAuthSession,
  clearServerAuthSession,
  getCurrentAuthState,
  loginWithAccessToken,
  loginWithUsername,
  registerWithCredentials,
  type AuthLoginMethods,
  type AuthSession,
} from "@/lib/auth/client";

export type AuthStoreState = {
  initialized: boolean;
  loginMethods: AuthLoginMethods;
  session: AuthSession | null;
};

type AuthStoreActions = {
  login: (username: string, password: string) => Promise<AuthSession>;
  loginWithToken: (token: string) => Promise<AuthSession>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  register: (
    username: string,
    email: string,
    code: string,
    password: string,
  ) => Promise<AuthSession>;
  sendCode: (email: string, scene: string) => Promise<void>;
  resetPassword: (
    email: string,
    code: string,
    password: string,
  ) => Promise<void>;
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
  login: async (username, password) => {
    const nextSession = await loginWithUsername(username, password);
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
  register: async (username, email, code, password) => {
    const nextSession = await registerWithCredentials(
      username,
      email,
      code,
      password,
    );
    set({ initialized: true, session: nextSession });
    return nextSession;
  },
  sendCode: async (email, scene) => {
    const { sendAuthCode } = await import("@/lib/auth/client");
    await sendAuthCode(email, scene);
  },
  resetPassword: async (email, code, password) => {
    const { resetPassword } = await import("@/lib/auth/client");
    await resetPassword(email, code, password);
  },
  reset: () => set(initialState),
}));
