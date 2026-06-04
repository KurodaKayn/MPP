// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  createCollabDocument,
  createCollabDocumentSession,
  listCollabDocuments,
  updateCollabDocument,
} from "./collab";

function jsonResponse(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    ...init,
  });
}

describe("collab api client", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("lists collaborative documents with pagination", async () => {
    const response = {
      items: [],
      limit: 10,
      page: 2,
      total: 0,
      total_pages: 0,
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(response));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listCollabDocuments({ limit: 10, page: 2 })).resolves.toEqual(
      response,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/collab/documents?page=2&limit=10",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
      }),
    );
  });

  it("creates collaborative documents", async () => {
    const document = {
      created_at: "2026-06-03T12:00:00Z",
      current_seq: 0,
      id: "doc-1",
      owner_user_id: "user-1",
      schema_version: 1,
      status: "active",
      title: "Launch Notes",
      updated_at: "2026-06-03T12:00:00Z",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(document));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      createCollabDocument({ title: "Launch Notes" }),
    ).resolves.toEqual(document);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/collab/documents",
      expect.objectContaining({
        body: JSON.stringify({ title: "Launch Notes" }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });

  it("updates document titles with encoded IDs", async () => {
    const document = {
      created_at: "2026-06-03T12:00:00Z",
      current_seq: 5,
      id: "doc/1",
      owner_user_id: "user-1",
      schema_version: 1,
      status: "active",
      title: "Renamed",
      updated_at: "2026-06-03T12:10:00Z",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(document));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      updateCollabDocument("doc/1", { title: "Renamed" }),
    ).resolves.toEqual(document);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/collab/documents/doc%2F1",
      expect.objectContaining({
        body: JSON.stringify({ title: "Renamed" }),
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "PATCH",
      }),
    );
  });

  it("requests collaboration sessions", async () => {
    const session = {
      document_id: "doc-1",
      expires_at: "2026-06-03T12:05:00Z",
      limits: {
        heartbeat_seconds: 30,
        max_message_bytes: 524288,
      },
      role: "editor",
      token: "short-lived-token",
      websocket_url: "ws://localhost:8090/collab/documents/doc-1",
    };
    const fetchMock = vi.fn<typeof fetch>(async () => jsonResponse(session));
    vi.stubGlobal("fetch", fetchMock);

    await expect(createCollabDocumentSession("doc-1")).resolves.toEqual(
      session,
    );

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/collab/documents/doc-1/session",
      expect.objectContaining({
        credentials: "same-origin",
        headers: expect.any(Headers),
        method: "POST",
      }),
    );
  });
});
