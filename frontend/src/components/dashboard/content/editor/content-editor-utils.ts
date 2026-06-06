import type { ContentValue } from "@/lib/content/types";
import { serializeMediaAssetRefs } from "./content-editor-media";

const EMPTY_DOCUMENT_HTML = "<p></p>";

export const MAX_INLINE_IMAGE_SIZE = 8 * 1024 * 1024;

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

  return documentFragment.body.innerHTML.trim() || EMPTY_DOCUMENT_HTML;
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

  documentFragment
    .querySelectorAll("script, style, iframe, object, embed")
    .forEach((element) => element.remove());

  documentFragment.querySelectorAll("*").forEach((element) => {
    [...element.attributes].forEach((attribute) => {
      if (attribute.name.toLowerCase().startsWith("on")) {
        element.removeAttribute(attribute.name);
      }
    });
  });

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
