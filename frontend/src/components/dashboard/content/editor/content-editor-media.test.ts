// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import {
  collectMediaAssetIds,
  hydrateMediaAssetRefs,
  serializeMediaAssetRefs,
} from "./content-editor-media";
import { contentValueFromHtml } from "./content-editor-utils";

describe("content editor media refs", () => {
  it("collects media asset ids from stable refs and data attributes", () => {
    const html = `
      <p>body</p>
      <img src="mpp://media/asset-1" alt="one">
      <img src="https://cdn.example/two.png" data-mpp-media-id="asset-2">
      <img src="mpp://media/asset-1" alt="duplicate">
    `;

    expect(collectMediaAssetIds(html)).toEqual(["asset-1", "asset-2"]);
  });

  it("hydrates stable media refs to signed preview URLs", () => {
    const html =
      '<p><img src="mpp://media/asset-1" data-mpp-media-id="asset-1" alt="cover"></p>';

    expect(
      hydrateMediaAssetRefs(html, [
        {
          asset_id: "asset-1",
          expires_at: "2026-06-05T12:05:00Z",
          url: "https://r2.example/signed-cover",
        },
      ]),
    ).toBe(
      '<p><img src="https://r2.example/signed-cover" data-mpp-media-id="asset-1" alt="cover"></p>',
    );
  });

  it("serializes hydrated preview URLs back to stable refs", () => {
    const html =
      '<p><img src="https://r2.example/signed-cover" data-mpp-media-id="asset-1" alt="cover"></p>';

    expect(serializeMediaAssetRefs(html)).toBe(
      '<p><img src="mpp://media/asset-1" data-mpp-media-id="asset-1" alt="cover"></p>',
    );
  });

  it("stores stable media refs when deriving content values", () => {
    const content = contentValueFromHtml(
      '<p><img src="https://r2.example/signed-cover" data-mpp-media-id="asset-1" alt="cover"></p>',
    );

    expect(content.firstImageSrc).toBe("mpp://media/asset-1");
    expect(content.html).toBe(
      '<p><img src="mpp://media/asset-1" data-mpp-media-id="asset-1" alt="cover"></p>',
    );
  });
});
