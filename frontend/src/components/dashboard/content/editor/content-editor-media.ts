import type { ResolvedMediaAsset } from "@/lib/dashboard/api";

const MEDIA_OBJECT_REF_PREFIX = "mpp://media/";

function canUseDomParser() {
  return typeof window !== "undefined" && typeof DOMParser !== "undefined";
}

export function mediaObjectRef(assetId: string) {
  return `${MEDIA_OBJECT_REF_PREFIX}${assetId}`;
}

function assetIdFromMediaRef(value: string | null | undefined) {
  if (!value?.startsWith(MEDIA_OBJECT_REF_PREFIX)) {
    return "";
  }

  return value.slice(MEDIA_OBJECT_REF_PREFIX.length).trim();
}

function assetIdFromImage(image: HTMLImageElement) {
  return (
    image.getAttribute("data-mpp-media-id") ||
    assetIdFromMediaRef(image.getAttribute("src")) ||
    ""
  );
}

export function collectMediaAssetIds(html: string) {
  if (!canUseDomParser()) {
    return [];
  }

  const ids = new Set<string>();
  const documentFragment = new DOMParser().parseFromString(html, "text/html");

  documentFragment.querySelectorAll("img").forEach((image) => {
    const assetId = assetIdFromImage(image);

    if (assetId) {
      ids.add(assetId);
    }
  });

  return Array.from(ids);
}

export function hydrateMediaAssetRefs(
  html: string,
  resolvedAssets: ResolvedMediaAsset[],
) {
  if (!canUseDomParser() || resolvedAssets.length === 0) {
    return html;
  }

  const previewUrls = new Map(
    resolvedAssets.map((asset) => [asset.asset_id, asset.url]),
  );
  const documentFragment = new DOMParser().parseFromString(html, "text/html");

  documentFragment.querySelectorAll("img").forEach((image) => {
    const assetId = assetIdFromImage(image);
    const previewUrl = previewUrls.get(assetId);

    if (!assetId || !previewUrl) {
      return;
    }

    image.setAttribute("data-mpp-media-id", assetId);
    image.setAttribute("src", previewUrl);
  });

  return documentFragment.body.innerHTML.trim() || html;
}

export function serializeMediaAssetRefs(html: string) {
  if (!canUseDomParser()) {
    return html;
  }

  const documentFragment = new DOMParser().parseFromString(html, "text/html");

  documentFragment.querySelectorAll("img").forEach((image) => {
    const assetId = assetIdFromImage(image);

    if (!assetId) {
      return;
    }

    image.setAttribute("data-mpp-media-id", assetId);
    image.setAttribute("src", mediaObjectRef(assetId));
  });

  return documentFragment.body.innerHTML.trim() || html;
}
