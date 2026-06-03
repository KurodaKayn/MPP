import { beforeEach, describe, expect, it, vi } from "vitest";
import { assetToFile } from "./douyin-dynamic";
import type { HandoffAsset } from "../types/handoff";

const asset: HandoffAsset = {
  type: "image",
  source_url: "https://assets.example.com/douyin-cover.png",
  name: "douyin-cover.png",
  mime_type: "image/png",
};

describe("assetToFile", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it("requests asset downloads through the extension background", async () => {
    const sendMessage = vi.fn(() =>
      Promise.resolve({
        name: "douyin-cover.png",
        mime_type: "image/png",
        data_base64: "SGVsbG8gRG91eWlu",
      }),
    );
    const fetchMock = vi.fn();

    vi.stubGlobal("browser", {
      runtime: {
        sendMessage,
      },
    });
    vi.stubGlobal("fetch", fetchMock);

    const file = await assetToFile(asset);
    const text = new TextDecoder().decode(await file.arrayBuffer());

    expect(sendMessage).toHaveBeenCalledWith({
      type: "asset.download",
      asset,
    });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(file.name).toBe("douyin-cover.png");
    expect(file.type).toBe("image/png");
    expect(text).toBe("Hello Douyin");
  });

  it("surfaces background download errors", async () => {
    vi.stubGlobal("browser", {
      runtime: {
        sendMessage: vi.fn(() =>
          Promise.resolve({
            error: "Asset download failed with HTTP 403.",
          }),
        ),
      },
    });

    await expect(assetToFile(asset)).rejects.toThrow(
      "Asset download failed with HTTP 403.",
    );
  });
});
