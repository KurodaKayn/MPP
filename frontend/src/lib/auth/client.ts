"use client";

import {
  authTokenNames,
  formatBearerToken,
  primaryAuthTokenName,
} from "./tokens";

export { formatBearerToken } from "./tokens";

export type AuthSession = {
  token: string | null;
  username: string;
};

export type AuthLoginMethods = {
  mock: boolean;
  token: boolean;
};

export type AuthState = {
  loginMethods: AuthLoginMethods;
  session: AuthSession | null;
};

type LoginResponse = {
  token?: string;
  message?: string;
  error?: {
    code?: string;
    message?: string;
  };
};

type AuthStatusResponse = {
  authenticated?: boolean;
  username?: string;
  loginMethods?: Partial<AuthLoginMethods>;
};

const defaultLoginMethods: AuthLoginMethods = {
  mock: false,
  token: true,
};
const authUserStorageKey = "sevenoxcloud.auth_user";
const authChangedEvent = "sevenoxcloud.auth_changed";
const cookieSessionUsername = "Creator";

function getStorageToken(storage: Storage) {
  for (const key of authTokenNames) {
    const token = storage.getItem(key);
    if (token) {
      return token;
    }
  }

  return null;
}

function getAuthUser(storage: Storage) {
  try {
    const raw = storage.getItem(authUserStorageKey);
    if (!raw) {
      return null;
    }

    const parsed = JSON.parse(raw) as Partial<AuthSession>;
    return typeof parsed.username === "string" ? parsed.username : null;
  } catch {
    return null;
  }
}

export function getStoredAuthToken() {
  if (typeof window === "undefined") {
    return null;
  }

  for (const getStorage of [
    () => window.localStorage,
    () => window.sessionStorage,
  ]) {
    try {
      const token = getStorageToken(getStorage());
      if (token) {
        return token;
      }
    } catch {
      // Some privacy modes can deny Web Storage access.
    }
  }

  return null;
}

export function getStoredAuthSession(): AuthSession | null {
  if (typeof window === "undefined") {
    return null;
  }

  const token = getStoredAuthToken();
  if (!token) {
    return null;
  }

  for (const getStorage of [
    () => window.localStorage,
    () => window.sessionStorage,
  ]) {
    try {
      const username = getAuthUser(getStorage());
      if (username) {
        return { token, username };
      }
    } catch {
      // Some privacy modes can deny Web Storage access.
    }
  }

  return { token, username: "Creator" };
}

async function getResponseErrorMessage(response: Response, fallback: string) {
  try {
    const body = (await response.json()) as LoginResponse;
    return body.error?.message || body.error?.code || body.message || fallback;
  } catch {
    return fallback;
  }
}

export async function getAuthStatus(): Promise<{
  authenticated: boolean;
  loginMethods: AuthLoginMethods;
  username: string | null;
}> {
  try {
    const response = await fetch("/api/auth/session", {
      cache: "no-store",
      credentials: "same-origin",
    });

    if (!response.ok) {
      return {
        authenticated: false,
        loginMethods: defaultLoginMethods,
        username: null,
      };
    }

    const body = (await response.json()) as AuthStatusResponse;
    return {
      authenticated: body.authenticated === true,
      loginMethods: {
        mock: body.loginMethods?.mock === true,
        token: body.loginMethods?.token ?? defaultLoginMethods.token,
      },
      username: typeof body.username === "string" ? body.username : null,
    };
  } catch {
    return {
      authenticated: false,
      loginMethods: defaultLoginMethods,
      username: null,
    };
  }
}

export async function getCurrentAuthState(): Promise<AuthState> {
  const storedSession = getStoredAuthSession();
  const status = await getAuthStatus();

  if (storedSession) {
    return {
      loginMethods: status.loginMethods,
      session: storedSession,
    };
  }

  if (status.authenticated) {
    return {
      loginMethods: status.loginMethods,
      session: {
        token: null,
        username: status.username ?? cookieSessionUsername,
      },
    };
  }

  return {
    loginMethods: status.loginMethods,
    session: null,
  };
}

function setAuthCookie(token: string) {
  document.cookie = `${primaryAuthTokenName}=${encodeURIComponent(
    token,
  )}; path=/; max-age=259200; SameSite=Lax`;
}

function clearAuthCookie() {
  document.cookie = `${primaryAuthTokenName}=; path=/; max-age=0; SameSite=Lax`;
}

function notifyAuthChanged() {
  window.dispatchEvent(new Event(authChangedEvent));
}

export function subscribeAuthChanged(listener: () => void) {
  window.addEventListener(authChangedEvent, listener);
  window.addEventListener("storage", listener);

  return () => {
    window.removeEventListener(authChangedEvent, listener);
    window.removeEventListener("storage", listener);
  };
}

export function setAuthSession(session: { token: string; username: string }) {
  window.localStorage.setItem(primaryAuthTokenName, session.token);
  window.localStorage.setItem(
    authUserStorageKey,
    JSON.stringify({ username: session.username }),
  );
  setAuthCookie(session.token);
  notifyAuthChanged();
}

export function clearAuthSession(options: { notify?: boolean } = {}) {
  for (const key of authTokenNames) {
    window.localStorage.removeItem(key);
    window.sessionStorage.removeItem(key);
  }
  window.localStorage.removeItem(authUserStorageKey);
  window.sessionStorage.removeItem(authUserStorageKey);
  clearAuthCookie();
  if (options.notify ?? true) {
    notifyAuthChanged();
  }
}

export async function clearServerAuthSession() {
  await fetch("/api/auth/session", {
    cache: "no-store",
    credentials: "same-origin",
    method: "DELETE",
  }).catch(() => undefined);
}

async function verifyAuthToken(token: string) {
  const headers = new Headers({
    Accept: "application/json",
    Authorization: formatBearerToken(token),
  });
  const response = await fetch("/api/user/dashboard/stats", {
    cache: "no-store",
    credentials: "same-origin",
    headers,
  });

  if (!response.ok) {
    throw new Error(
      await getResponseErrorMessage(response, `登录失败 (${response.status})`),
    );
  }
}

export async function loginWithAccessToken(token: string) {
  const normalizedToken = token.trim();

  if (!normalizedToken) {
    throw new Error("请输入访问令牌");
  }

  await verifyAuthToken(normalizedToken);

  const session = { token: normalizedToken, username: cookieSessionUsername };
  setAuthSession(session);
  return session;
}

export async function loginWithUsername(username: string) {
  const normalizedUsername = username.trim();
  if (!normalizedUsername) {
    throw new Error("请输入用户名");
  }

  const status = await getAuthStatus();
  if (!status.loginMethods.mock) {
    throw new Error("开发账号登录仅在本地开发环境可用");
  }

  const response = await fetch("/api/auth/login", {
    body: JSON.stringify({ username: normalizedUsername }),
    cache: "no-store",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  });
  const body = (await response.json().catch(() => ({}))) as LoginResponse;

  if (!response.ok || !body.token) {
    throw new Error(
      body.error?.message || body.error?.code || body.message || "登录失败",
    );
  }

  const session = { token: body.token, username: normalizedUsername };
  setAuthSession(session);
  return session;
}
