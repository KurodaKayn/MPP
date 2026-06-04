import { describe, expect, it, vi } from "vitest";
import {
  BackendApiError,
  createBackendClient,
  normalizeBackendError,
} from "./client";
import { resolveBackendConfig } from "./config";

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status: init?.status ?? 200,
    statusText: init?.statusText,
  });
}

describe("resolveBackendConfig", () => {
  it("uses the local backend API when no base URL is configured", () => {
    expect(resolveBackendConfig({}).apiBaseUrl).toBe("http://localhost:8080");
  });

  it("normalizes the configured API base URL", () => {
    expect(
      resolveBackendConfig({
        WXT_MPP_API_BASE_URL: " https://mpp.example.com/api/ ",
      }).apiBaseUrl,
    ).toBe("https://mpp.example.com/api");
  });
});

describe("createBackendClient", () => {
  it("fetches the extension session with bearer auth and credentials compatibility", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        authenticated: true,
        user: {
          id: "user-1",
          username: "creator",
        },
      }),
    );
    const client = createBackendClient({
      apiBaseUrl: "https://mpp.example.com",
      authTokenProvider: () => "jwt-token",
      fetch: fetchMock,
    });

    const session = await client.getSession();
    const [, requestInit] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(requestInit.headers);

    expect(session.user.username).toBe("creator");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://mpp.example.com/api/user/dashboard/extension/session",
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("Authorization")).toBe("Bearer jwt-token");
  });

  it("fetches pre-publish items from the backend", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        items: [
          {
            project_id: "project-1",
            title: "Draft title",
            status: "ready",
            updated_at: "2026-06-03T10:00:00Z",
            platforms: [
              {
                publication_id: "publication-1",
                platform: "douyin",
                adapter_key: "DYNAMIC_DOUYIN",
                content_kind: "article",
                status: "adapted",
                enabled: true,
                preview: "Preview",
              },
            ],
          },
        ],
      }),
    );
    const client = createBackendClient({
      apiBaseUrl: "https://mpp.example.com/",
      authTokenProvider: async () => "jwt-token",
      fetch: fetchMock,
    });

    const prepublish = await client.listPrepublish();

    expect(prepublish.items).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "https://mpp.example.com/api/user/dashboard/extension/prepublish",
      expect.objectContaining({
        method: "GET",
      }),
    );
  });

  it("requests a backend-generated handoff for selected platforms", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        schema_version: 1,
        type: "mpp.extension_publish_handoff",
        execution_id: "execution-1",
        expires_at: "2026-06-03T10:10:00Z",
        project: {
          id: "project-1",
          title: "Draft title",
        },
        platforms: [],
      }),
    );
    const client = createBackendClient({
      apiBaseUrl: "https://mpp.example.com",
      authTokenProvider: () => "jwt-token",
      fetch: fetchMock,
    });

    const handoff = await client.createHandoff({
      project_id: "project-1",
      platforms: ["douyin"],
    });
    const [, requestInit] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = new Headers(requestInit.headers);

    expect(handoff.execution_id).toBe("execution-1");
    expect(fetchMock).toHaveBeenCalledWith(
      "https://mpp.example.com/api/user/dashboard/extension/handoffs",
      expect.objectContaining({
        body: JSON.stringify({
          project_id: "project-1",
          platforms: ["douyin"],
        }),
        method: "POST",
      }),
    );
    expect(headers.get("Content-Type")).toBe("application/json");
  });

  it("requires a bearer token before calling the backend", async () => {
    const client = createBackendClient({
      apiBaseUrl: "https://mpp.example.com",
      authTokenProvider: () => "",
      fetch: vi.fn(),
    });

    await expect(client.getSession()).rejects.toMatchObject({
      code: "missing_auth_token",
      status: 401,
    });
  });

  it("normalizes backend error responses", async () => {
    const client = createBackendClient({
      apiBaseUrl: "https://mpp.example.com",
      authTokenProvider: () => "jwt-token",
      fetch: vi.fn().mockResolvedValue(
        jsonResponse(
          {
            error: {
              code: "unauthorized",
              message: "token expired",
            },
          },
          { status: 401, statusText: "Unauthorized" },
        ),
      ),
    });

    await expect(client.getSession()).rejects.toMatchObject({
      code: "unauthorized",
      message: "token expired",
      status: 401,
    });
  });

  it("normalizes network failures", () => {
    const error = normalizeBackendError(new TypeError("Failed to fetch"));

    expect(error).toBeInstanceOf(BackendApiError);
    expect(error.code).toBe("network_error");
    expect(error.status).toBe(0);
  });
});
