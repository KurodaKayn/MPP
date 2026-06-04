const defaultApiBaseUrl = "http://localhost:8080";
const defaultWebBaseUrl = "http://localhost:3000";
const apiBaseUrlEnvKey = "WXT_MPP_API_BASE_URL";
const webBaseUrlEnvKey = "WXT_MPP_WEB_BASE_URL";

export interface BackendConfig {
  apiBaseUrl: string;
  loginUrl: string;
  webBaseUrl: string;
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
  const configuredWebBaseUrl = env[webBaseUrlEnvKey];
  const webBaseUrl = normalizeBaseUrl(
    configuredWebBaseUrl && configuredWebBaseUrl.trim() !== ""
      ? configuredWebBaseUrl
      : defaultWebBaseUrl,
  );

  return {
    apiBaseUrl,
    loginUrl: `${webBaseUrl}/zh/login`,
    webBaseUrl,
  };
}

export const backendConfig = resolveBackendConfig(import.meta.env);
