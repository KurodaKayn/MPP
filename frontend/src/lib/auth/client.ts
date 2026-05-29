"use client";

export type AuthSession = {
  token: string;
  username: string;
};

type LoginResponse = {
  token?: string;
  message?: string;
  error?: {
    code?: string;
    message?: string;
  };
};

const authTokenStorageKeys = [
  "sevenoxcloud.auth_token",
  "auth_token",
  "access_token",
] as const;
const primaryAuthTokenStorageKey = authTokenStorageKeys[0];
const authUserStorageKey = "sevenoxcloud.auth_user";
const authChangedEvent = "sevenoxcloud.auth_changed";

export function formatBearerToken(token: string) {
  return token.toLowerCase().startsWith("bearer ") ? token : `Bearer ${token}`;
}

function getStorageToken(storage: Storage) {
  for (const key of authTokenStorageKeys) {
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

function setAuthCookie(token: string) {
  document.cookie = `${primaryAuthTokenStorageKey}=${encodeURIComponent(
    token,
  )}; path=/; max-age=259200; SameSite=Lax`;
}

function clearAuthCookie() {
  document.cookie = `${primaryAuthTokenStorageKey}=; path=/; max-age=0; SameSite=Lax`;
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

export function setAuthSession(session: AuthSession) {
  window.localStorage.setItem(primaryAuthTokenStorageKey, session.token);
  window.localStorage.setItem(
    authUserStorageKey,
    JSON.stringify({ username: session.username }),
  );
  setAuthCookie(session.token);
  notifyAuthChanged();
}

export function clearAuthSession() {
  for (const key of authTokenStorageKeys) {
    window.localStorage.removeItem(key);
    window.sessionStorage.removeItem(key);
  }
  window.localStorage.removeItem(authUserStorageKey);
  window.sessionStorage.removeItem(authUserStorageKey);
  clearAuthCookie();
  notifyAuthChanged();
}

export async function loginWithUsername(username: string) {
  const response = await fetch("/api/auth/mock-login", {
    body: JSON.stringify({ username }),
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

  const session = { token: body.token, username };
  setAuthSession(session);
  return session;
}
