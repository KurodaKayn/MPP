import { describe, expect, it } from "vitest";
import {
  getCollabUserProfile,
  renderCollabSelection,
  resolveCollabWebSocketUrl,
} from "./collab-provider";

describe("collab provider utilities", () => {
  it("keeps absolute websocket URLs unchanged", () => {
    expect(
      resolveCollabWebSocketUrl(
        "ws://localhost:8090/collab/documents/doc-1",
        "https://app.example",
      ),
    ).toBe("ws://localhost:8090/collab/documents/doc-1");
  });

  it("resolves relative websocket URLs from the current origin", () => {
    expect(
      resolveCollabWebSocketUrl(
        "/collab/documents/doc-1",
        "https://app.example",
      ),
    ).toBe("wss://app.example/collab/documents/doc-1");

    expect(
      resolveCollabWebSocketUrl("/collab/documents/doc-1", "http://localhost"),
    ).toBe("ws://localhost/collab/documents/doc-1");
  });

  it("creates stable user profiles for the same seed and role", () => {
    const first = getCollabUserProfile("  Kuroda  ", "editor", "doc-1");
    const second = getCollabUserProfile("Kuroda", "editor", "doc-1");

    expect(first).toEqual(second);
    expect(first.name).toBe("Kuroda");
    expect(first.role).toBe("editor");
    expect(first.color).toMatch(/^#[0-9a-f]{6}$/i);
  });

  it("renders selection decoration attributes", () => {
    expect(
      renderCollabSelection({
        color: "#2563eb",
        name: "Reviewer",
      }),
    ).toEqual({
      "data-user": "Reviewer",
      nodeName: "span",
      style: "background-color: #2563eb30",
    });
  });
});
