export const extensionAuthTokenStorageKeys = [
  "sevenoxcloud.auth_token",
  "auth_token",
  "access_token",
  "mpp.web_auth_token",
] as const;

export interface ExtensionAuthTokenStorage {
  get(keys: readonly string[]): Promise<Record<string, unknown>>;
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
