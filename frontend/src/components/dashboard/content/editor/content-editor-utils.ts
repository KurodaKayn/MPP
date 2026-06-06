import type { ContentValue } from "@/lib/content/types";
import { serializeMediaAssetRefs } from "./content-editor-media";

const EMPTY_DOCUMENT_HTML = "<p></p>";

export const MAX_INLINE_IMAGE_SIZE = 8 * 1024 * 1024;
const BLOCKED_HTML_ELEMENTS =
  "base, embed, form, iframe, link, math, meta, object, script, style, svg, template";
const URL_ATTRIBUTES = new Set([
  "action",
  "formaction",
  "href",
  "poster",
  "src",
  "xlink:href",
]);
const BLOCKED_ATTRIBUTES = new Set(["srcdoc", "srcset", "style"]);

function canUseDomParser() {
  return typeof window !== "undefined" && typeof DOMParser !== "undefined";
}

export function normalizeStoredHtml(html: string) {
  const source = html.trim() ? html : EMPTY_DOCUMENT_HTML;

  if (!canUseDomParser()) {
    return source;
  }

  const documentFragment = new DOMParser().parseFromString(source, "text/html");

  documentFragment.querySelectorAll("figure").forEach((figure) => {
    const image = figure.querySelector("img");
    const caption = figure.querySelector("figcaption")?.textContent?.trim();
    const fragment = documentFragment.createDocumentFragment();

    if (image?.getAttribute("src")) {
      const nextImage = documentFragment.createElement("img");
      nextImage.setAttribute("src", image.getAttribute("src") ?? "");
      const mediaAssetId = image.getAttribute("data-mpp-media-id");
      if (mediaAssetId) {
        nextImage.setAttribute("data-mpp-media-id", mediaAssetId);
      }
      nextImage.setAttribute(
        "alt",
        image.getAttribute("alt") ?? caption ?? "Image",
      );
      fragment.append(nextImage);
    }

    if (caption) {
      const captionParagraph = documentFragment.createElement("p");
      captionParagraph.textContent = caption;
      fragment.append(captionParagraph);
    }

    figure.replaceWith(fragment);
  });

  return (
    sanitizeStoredHtml(documentFragment.body.innerHTML) || EMPTY_DOCUMENT_HTML
  );
}

export function normalizeUrl(url: string) {
  const trimmedUrl = url.trim();

  if (!trimmedUrl) {
    return "";
  }

  const withProtocol = /^https?:\/\//i.test(trimmedUrl)
    ? trimmedUrl
    : `https://${trimmedUrl}`;

  try {
    const parsed = new URL(withProtocol);
    return ["http:", "https:"].includes(parsed.protocol)
      ? parsed.toString()
      : "";
  } catch {
    return "";
  }
}

export function sanitizeClipboardHtml(html: string) {
  if (!canUseDomParser()) {
    return html;
  }

  const documentFragment = new DOMParser().parseFromString(html, "text/html");

  documentFragment.querySelectorAll("a").forEach((anchor) => {
    const safeHref = normalizeUrl(anchor.getAttribute("href") ?? "");

    if (!safeHref) {
      anchor.replaceWith(
        documentFragment.createTextNode(anchor.textContent ?? ""),
      );
      return;
    }

    anchor.setAttribute("href", safeHref);
    anchor.setAttribute("target", "_blank");
    anchor.setAttribute("rel", "noopener noreferrer");
  });

  documentFragment.querySelectorAll("img").forEach((image) => {
    const src = image.getAttribute("src") ?? "";

    if (!/^(https?:|data:image\/|blob:|mpp:\/\/media\/)/i.test(src)) {
      image.remove();
    }
  });

  return normalizeStoredHtml(documentFragment.body.innerHTML);
}

export function sanitizeStoredHtml(html: string) {
  if (!canUseDomParser()) {
    return html.trim();
  }

  const documentFragment = new DOMParser().parseFromString(html, "text/html");

  documentFragment
    .querySelectorAll(BLOCKED_HTML_ELEMENTS)
    .forEach((element) => element.remove());

  documentFragment.querySelectorAll("*").forEach((element) => {
    [...element.attributes].forEach((attribute) => {
      const attributeName = attribute.name.toLowerCase();
      if (
        attributeName.startsWith("on") ||
        attributeName.startsWith("xmlns") ||
        BLOCKED_ATTRIBUTES.has(attributeName)
      ) {
        element.removeAttribute(attribute.name);
        return;
      }

      if (
        URL_ATTRIBUTES.has(attributeName) &&
        !isSafeHtmlUrl(attributeName, attribute.value)
      ) {
        element.removeAttribute(attribute.name);
      }
    });
  });

  return documentFragment.body.innerHTML.trim();
}

function isSafeHtmlUrl(attributeName: string, value: string) {
  const normalizedValue = normalizeHtmlUrlForSafety(value);
  if (!normalizedValue) {
    return false;
  }

  if (normalizedValue.startsWith("#") || normalizedValue.startsWith("/")) {
    return true;
  }

  if (!normalizedValue.includes(":")) {
    return true;
  }

  const scheme = normalizedValue.split(":", 1)[0];
  if (attributeName === "href" || attributeName === "xlink:href") {
    return ["http", "https", "mailto", "tel"].includes(scheme);
  }

  if (attributeName === "src" || attributeName === "poster") {
    return (
      ["http", "https", "blob"].includes(scheme) ||
      normalizedValue.startsWith("mpp://media/") ||
      isSafeDataImageUrl(normalizedValue)
    );
  }

  return ["http", "https"].includes(scheme);
}

function normalizeHtmlUrlForSafety(value: string) {
  const textArea = document.createElement("textarea");
  textArea.innerHTML = value.trim();
  let normalizedValue = "";

  for (const character of textArea.value) {
    const codePoint = character.codePointAt(0) ?? 0;
    if (codePoint <= 0x1f || codePoint === 0x7f || /\s/.test(character)) {
      continue;
    }

    normalizedValue += character;
  }

  return normalizedValue.toLowerCase();
}

function isSafeDataImageUrl(value: string) {
  return /^data:image\/(?:png|jpe?g|gif|webp|avif);base64,/i.test(value);
}

export function contentValueFromHtml(html: string): ContentValue {
  const storedHtml = serializeMediaAssetRefs(html);

  if (!canUseDomParser()) {
    return {
      firstImageSrc: "",
      html: storedHtml,
      text: "",
    };
  }

  const documentFragment = new DOMParser().parseFromString(
    storedHtml,
    "text/html",
  );

  return {
    firstImageSrc:
      documentFragment.querySelector("img")?.getAttribute("src") ?? "",
    html: storedHtml,
    text:
      documentFragment.body.innerText?.trim() ||
      documentFragment.body.textContent?.trim() ||
      "",
  };
}
