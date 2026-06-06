// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import {
  normalizeStoredHtml,
  sanitizeStoredHtml,
} from "./content-editor-utils";

function parseHTML(html: string) {
  return new DOMParser().parseFromString(html, "text/html");
}

describe("content editor HTML sanitization", () => {
  it("removes active content from stored HTML", () => {
    const sanitized = normalizeStoredHtml(`
      <p onclick="alert(1)">Hello <strong>safe</strong></p>
      <script>alert(1)</script>
      <img src="javascript:alert(1)" onerror="alert(1)" alt="cover">
      <a href="java&#x0A;script:alert(1)">bad link</a>
      <a data-testid="tab-link" href="java&#x09;script:alert(1)">tab link</a>
      <svg onload="alert(1)"><circle /></svg>
    `);
    const documentFragment = parseHTML(sanitized);

    expect(documentFragment.querySelector("p")?.getAttribute("onclick")).toBe(
      null,
    );
    expect(documentFragment.querySelector("script")).toBe(null);
    expect(documentFragment.querySelector("svg")).toBe(null);
    expect(documentFragment.querySelector("img")?.getAttribute("src")).toBe(
      null,
    );
    expect(documentFragment.querySelector("img")?.getAttribute("onerror")).toBe(
      null,
    );
    expect(documentFragment.querySelector("a")?.getAttribute("href")).toBe(
      null,
    );
    expect(
      documentFragment
        .querySelector('[data-testid="tab-link"]')
        ?.getAttribute("href"),
    ).toBe(null);
  });

  it("keeps safe links and media refs", () => {
    const sanitized = sanitizeStoredHtml(`
      <a href="https://example.com/post">safe</a>
      <img src="mpp://media/asset-1" data-mpp-media-id="asset-1" alt="asset">
      <img src="data:image/png;base64,aGVsbG8=" alt="inline">
    `);
    const documentFragment = parseHTML(sanitized);
    const images = documentFragment.querySelectorAll("img");

    expect(documentFragment.querySelector("a")?.getAttribute("href")).toBe(
      "https://example.com/post",
    );
    expect(images[0]?.getAttribute("src")).toBe("mpp://media/asset-1");
    expect(images[0]?.getAttribute("data-mpp-media-id")).toBe("asset-1");
    expect(images[1]?.getAttribute("src")).toBe(
      "data:image/png;base64,aGVsbG8=",
    );
  });
});
