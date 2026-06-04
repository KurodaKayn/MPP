import { backendConfig, type BackendConfig } from "./config";
import type {
  BackendErrorPayload,
  BackendExtensionPublishHandoff,
  CreateExtensionHandoffRequest,
  ExtensionPrepublishResponse,
  ExtensionSessionResponse,
} from "./types";

const extensionDashboardPath = "/api/user/dashboard/extension";

type Fetcher = typeof fetch;
type AuthTokenProvider = () =>
  | Promise<string | null | undefined>
  | string
  | null
  | undefined;

export interface BackendClientOptions extends Partial<BackendConfig> {
  authTokenProvider: AuthTokenProvider;
  fetch?: Fetcher;
}

export interface BackendClient {
  getSession(): Promise<ExtensionSessionResponse>;
  listPrepublish(): Promise<ExtensionPrepublishResponse>;
  createHandoff(
    request: CreateExtensionHandoffRequest,
  ): Promise<BackendExtensionPublishHandoff>;
}

export class BackendApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, options: { code: string; status: number }) {
    super(message);
    this.name = "BackendApiError";
    this.code = options.code;
    this.status = options.status;
  }
}

function buildUrl(apiBaseUrl: string, path: string): string {
  return `${apiBaseUrl.replace(/\/+$/, "")}${path}`;
}

function isBackendApiError(value: unknown): value is BackendApiError {
  return value instanceof BackendApiError;
}

function isBackendErrorPayload(value: unknown): value is BackendErrorPayload {
  return typeof value === "object" && value !== null;
}

async function readResponseJson(response: Response): Promise<unknown> {
  const contentType = response.headers.get("Content-Type") ?? "";

  if (!contentType.toLowerCase().includes("application/json")) {
    return null;
  }

  return response.json();
}

async function readErrorResponse(response: Response): Promise<BackendApiError> {
  let payload: unknown = null;

  try {
    payload = await readResponseJson(response);
  } catch {
    payload = null;
  }

  const backendError = isBackendErrorPayload(payload) ? payload.error : null;
  const code = backendError?.code || `http_${response.status}`;
  const message =
    backendError?.message ||
    response.statusText ||
    `Backend request failed with HTTP ${response.status}.`;

  return new BackendApiError(message, {
    code,
    status: response.status,
  });
}

export function normalizeBackendError(error: unknown): BackendApiError {
  if (isBackendApiError(error)) {
    return error;
  }

  const message = error instanceof Error ? error.message : String(error);

  return new BackendApiError(message || "Backend request failed.", {
    code: "network_error",
    status: 0,
  });
}

export function createBackendClient(
  options: BackendClientOptions,
): BackendClient {
  const apiBaseUrl = (options.apiBaseUrl ?? backendConfig.apiBaseUrl).replace(
    /\/+$/,
    "",
  );
  const fetcher = options.fetch ?? fetch.bind(globalThis);

  async function getAuthToken(): Promise<string> {
    const token = (await options.authTokenProvider())?.trim() ?? "";

    if (!token) {
      throw new BackendApiError("MPP login token is unavailable.", {
        code: "missing_auth_token",
        status: 401,
      });
    }

    return token;
  }

  async function requestJson<T>(
    path: string,
    init: RequestInit = {},
  ): Promise<T> {
    try {
      const token = await getAuthToken();
      const headers = new Headers(init.headers);
      headers.set("Accept", "application/json");
      headers.set("Authorization", `Bearer ${token}`);

      const response = await fetcher(buildUrl(apiBaseUrl, path), {
        ...init,
        credentials: "include",
        headers,
      });

      if (!response.ok) {
        throw await readErrorResponse(response);
      }

      return (await readResponseJson(response)) as T;
    } catch (error) {
      throw normalizeBackendError(error);
    }
  }

  return {
    getSession: () =>
      requestJson<ExtensionSessionResponse>(
        `${extensionDashboardPath}/session`,
        { method: "GET" },
      ),
    listPrepublish: () =>
      requestJson<ExtensionPrepublishResponse>(
        `${extensionDashboardPath}/prepublish`,
        { method: "GET" },
      ),
    createHandoff: (request) =>
      requestJson<BackendExtensionPublishHandoff>(
        `${extensionDashboardPath}/handoffs`,
        {
          body: JSON.stringify(request),
          headers: {
            "Content-Type": "application/json",
          },
          method: "POST",
        },
      ),
  };
}
