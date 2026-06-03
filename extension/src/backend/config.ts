const defaultApiBaseUrl = "http://localhost:8080";
const apiBaseUrlEnvKey = "WXT_MPP_API_BASE_URL";

export interface BackendConfig {
  apiBaseUrl: string;
}

export type BackendConfigEnv = Readonly<Record<string, string | undefined>>;

function normalizeBaseUrl(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

export function resolveBackendConfig(env: BackendConfigEnv): BackendConfig {
  const configuredBaseUrl = env[apiBaseUrlEnvKey];
  const apiBaseUrl = normalizeBaseUrl(
    configuredBaseUrl && configuredBaseUrl.trim() !== ""
      ? configuredBaseUrl
      : defaultApiBaseUrl,
  );

  return { apiBaseUrl };
}

export const backendConfig = resolveBackendConfig(import.meta.env);
