import { storage } from "#imports";

export interface TrustedOrigin {
  origin: string;
  trusted_at: string;
}

const trustedOriginsItem = storage.defineItem<TrustedOrigin[]>(
  "local:mpp.trusted_origins",
  { fallback: [] },
);

export function normalizeOrigin(value: string): string | null {
  try {
    return new URL(value).origin;
  } catch {
    return null;
  }
}

export function isTrustableOrigin(origin: string): boolean {
  const normalized = normalizeOrigin(origin);

  if (!normalized) {
    return false;
  }

  const url = new URL(normalized);

  if (url.protocol === "http:") {
    return url.hostname === "localhost" || url.hostname === "127.0.0.1";
  }

  if (url.protocol !== "https:") {
    return false;
  }

  return (
    url.hostname === "mpp.example.com" ||
    url.hostname === "localhost" ||
    url.hostname === "127.0.0.1"
  );
}

export async function listTrustedOrigins(): Promise<TrustedOrigin[]> {
  return trustedOriginsItem.getValue();
}

export async function isTrustedOrigin(origin: string): Promise<boolean> {
  const normalized = normalizeOrigin(origin);

  if (!normalized || !isTrustableOrigin(normalized)) {
    return false;
  }

  const trustedOrigins = await trustedOriginsItem.getValue();
  return trustedOrigins.some((item) => item.origin === normalized);
}

export async function trustOrigin(origin: string): Promise<TrustedOrigin> {
  const normalized = normalizeOrigin(origin);

  if (!normalized || !isTrustableOrigin(normalized)) {
    throw new Error("Origin is not eligible for trust.");
  }

  const trustedOrigin: TrustedOrigin = {
    origin: normalized,
    trusted_at: new Date().toISOString(),
  };
  const trustedOrigins = await trustedOriginsItem.getValue();
  const nextTrustedOrigins = [
    ...trustedOrigins.filter((item) => item.origin !== normalized),
    trustedOrigin,
  ];

  await trustedOriginsItem.setValue(nextTrustedOrigins);
  return trustedOrigin;
}

export async function removeTrustedOrigin(
  origin: string,
): Promise<TrustedOrigin[]> {
  const normalized = normalizeOrigin(origin);

  if (!normalized) {
    throw new Error("Origin is invalid.");
  }

  const trustedOrigins = await trustedOriginsItem.getValue();
  const nextTrustedOrigins = trustedOrigins.filter(
    (item) => item.origin !== normalized,
  );

  await trustedOriginsItem.setValue(nextTrustedOrigins);
  return nextTrustedOrigins;
}

export function getTrustOriginPageUrl(origin: string): string {
  return browser.runtime.getURL(
    `/trust-origin.html?origin=${encodeURIComponent(origin)}`,
  );
}
