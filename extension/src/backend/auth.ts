export const extensionAuthTokenStorageKeys = [
  "sevenoxcloud.auth_token",
  "auth_token",
  "access_token",
  "mpp.web_auth_token",
] as const;

const canonicalExtensionAuthTokenStorageKey = extensionAuthTokenStorageKeys[0];

export interface ExtensionAuthTokenStorage {
  get(keys: readonly string[]): Promise<Record<string, unknown>>;
}

export interface ExtensionAuthTokenWritableStorage {
  set(values: Record<string, string>): Promise<void>;
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
