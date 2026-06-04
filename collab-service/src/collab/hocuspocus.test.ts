import { Hocuspocus } from "@hocuspocus/server";
import { describe, expect, it } from "vitest";

import { closeCollabServer } from "./hocuspocus.js";

import type { CollabConnectionContext } from "./hocuspocus.js";

describe("closeCollabServer", () => {
  it("waits for pending debounced Yjs stores before returning", async () => {
    const events: string[] = [];
    let markStoreStarted: () => void = () => {};
    let releaseStore: () => void = () => {};
    const storeStarted = new Promise<void>((resolve) => {
      markStoreStarted = resolve;
    });
    const storeCanFinish = new Promise<void>((resolve) => {
      releaseStore = resolve;
    });
    const collabServer = new Hocuspocus<CollabConnectionContext>({
      debounce: 60_000,
      maxDebounce: 60_000,
      quiet: true,
      async onStoreDocument() {
        events.push("store:start");
        markStoreStarted();
        await storeCanFinish;
        events.push("store:finish");
      },
    });
    const document = await collabServer.createDocument(
      "shutdown-doc",
      new Request("http://localhost"),
      "socket-id",
      {
        isAuthenticated: true,
        readOnly: false,
      },
    );
    document.getMap("content").set("title", "Saved before shutdown");

    const closePromise = closeCollabServer(collabServer).then(() => {
      events.push("closed");
    });

    await storeStarted;

    expect(events).toEqual(["store:start"]);

    releaseStore();
    await closePromise;

    expect(events).toEqual(["store:start", "store:finish", "closed"]);
    expect(collabServer.documents.size).toBe(0);
  });
});
