import { backendConfig } from "./config";

export const extensionAuthTokenStorageKeys = [
  "sevenoxcloud.auth_token",
  "auth_token",
  "access_token",
  "mpp.web_auth_token",
] as const;
const extensionAuthTokenCookieKeys = [
  "sevenoxcloud.auth_token",
  "auth_token",
  "access_token",
] as const;

const canonicalExtensionAuthTokenStorageKey = extensionAuthTokenStorageKeys[0];

export interface ExtensionAuthTokenStorage {
  get(keys: readonly string[]): Promise<Record<string, unknown>>;
}

export interface ExtensionAuthTokenWritableStorage {
  set(values: Record<string, string>): Promise<void>;
}

export interface ExtensionAuthTokenClearableStorage {
  remove(keys: string[]): Promise<void>;
}

export interface ExtensionAuthCookieStorage {
  get(details: {
    name: string;
    url: string;
  }): Promise<{ value?: string } | null | undefined>;
}

export interface ExtensionAuthCookieClearableStorage {
  remove(details: { name: string; url: string }): Promise<unknown>;
}

export interface ExtensionAuthTokenOptions {
  cookies?: ExtensionAuthCookieStorage | null;
  storage?: ExtensionAuthTokenStorage;
  webBaseUrl?: string;
}

export interface ExtensionAuthSessionClearOptions {
  cookies?: ExtensionAuthCookieClearableStorage | null;
  storage?: ExtensionAuthTokenClearableStorage;
  webBaseUrl?: string;
}

export interface WebAuthTokenStorage {
  getItem(key: string): string | null;
}

function normalizeToken(value: unknown): string | null {
  if (typeof value !== "string") {
    return null;
  }

  const token = value.trim();
  if (!token) {
    return null;
  }

  return token.toLowerCase().startsWith("bearer ")
    ? token.slice("bearer ".length).trim()
    : token;
}

export function getWebAuthTokenFromStorage(
  storage: WebAuthTokenStorage = window.localStorage,
): string | null {
  for (const key of extensionAuthTokenStorageKeys) {
    const token = normalizeToken(storage.getItem(key));

    if (token) {
      return token;
    }
  }

  return null;
}

export async function getStoredExtensionAuthToken(
  storage: ExtensionAuthTokenStorage = browser.storage.local,
): Promise<string | null> {
  const values = await storage.get(extensionAuthTokenStorageKeys);

  for (const key of extensionAuthTokenStorageKeys) {
    const token = normalizeToken(values[key]);
    if (token) {
      return token;
    }
  }

  return null;
}

function getDefaultCookieStorage(): ExtensionAuthCookieStorage | null {
  return browser.cookies ?? null;
}

function getDefaultCookieClearableStorage(): ExtensionAuthCookieClearableStorage | null {
  return browser.cookies ?? null;
}

function getCookieLookupUrl(webBaseUrl: string): string {
  try {
    return new URL(webBaseUrl).origin;
  } catch {
    return webBaseUrl;
  }
}

async function getExtensionAuthCookieToken(
  cookies: ExtensionAuthCookieStorage,
  webBaseUrl: string,
): Promise<string | null> {
  const url = getCookieLookupUrl(webBaseUrl);

  for (const name of extensionAuthTokenCookieKeys) {
    const cookie = await cookies.get({ name, url });
    const token = normalizeToken(cookie?.value);

    if (token) {
      return token;
    }
  }

  return null;
}

export async function getExtensionAuthToken(
  options: ExtensionAuthTokenOptions = {},
): Promise<string | null> {
  const cookies =
    "cookies" in options ? options.cookies : getDefaultCookieStorage();

  if (cookies) {
    const token = await getExtensionAuthCookieToken(
      cookies,
      options.webBaseUrl ?? backendConfig.webBaseUrl,
    ).catch(() => null);

    if (token) {
      return token;
    }
  }

  return getStoredExtensionAuthToken(options.storage);
}

export async function persistExtensionAuthToken(
  token: unknown,
  storage: ExtensionAuthTokenWritableStorage = browser.storage.local,
): Promise<boolean> {
  const normalizedToken = normalizeToken(token);

  if (!normalizedToken) {
    return false;
  }

  await storage.set({
    [canonicalExtensionAuthTokenStorageKey]: normalizedToken,
  });

  return true;
}

export async function clearStoredExtensionAuthTokens(
  storage: ExtensionAuthTokenClearableStorage = browser.storage.local,
): Promise<void> {
  await storage.remove([...extensionAuthTokenStorageKeys]);
}

export async function clearExtensionAuthSession(
  options: ExtensionAuthSessionClearOptions = {},
): Promise<void> {
  await clearStoredExtensionAuthTokens(options.storage);

  const cookies =
    "cookies" in options ? options.cookies : getDefaultCookieClearableStorage();

  if (!cookies) {
    return;
  }

  const url = getCookieLookupUrl(
    options.webBaseUrl ?? backendConfig.webBaseUrl,
  );

  await Promise.all(
    extensionAuthTokenCookieKeys.map((name) =>
      cookies.remove({ name, url }).catch(() => undefined),
    ),
  );
}
