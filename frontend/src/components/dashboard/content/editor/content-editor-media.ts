import type { ResolvedMediaAsset } from "@/lib/dashboard/api";

const LOCAL_MEDIA_ID_PREFIX = "local-";
const MEDIA_OBJECT_REF_PREFIX = "mpp://media/";

export type CompletedLocalMediaRef = {
  assetId: string;
  localMediaId: string;
};

function canUseDomParser() {
  return typeof window !== "undefined" && typeof DOMParser !== "undefined";
}

export function mediaObjectRef(assetId: string) {
  return `${MEDIA_OBJECT_REF_PREFIX}${assetId}`;
}

export function createLocalMediaId(value?: string) {
  const suffix =
    value?.trim() ||
    globalThis.crypto?.randomUUID?.() ||
    `${Date.now()}-${Math.random().toString(36).slice(2)}`;

  if (suffix.startsWith(LOCAL_MEDIA_ID_PREFIX)) {
    return suffix;
  }

  return `${LOCAL_MEDIA_ID_PREFIX}${suffix}`;
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

export function collectLocalMediaIds(html: string) {
  if (!canUseDomParser()) {
    return [];
  }

  const ids = new Set<string>();
  const documentFragment = new DOMParser().parseFromString(html, "text/html");

  documentFragment.querySelectorAll("img").forEach((image) => {
    const localMediaId = image.getAttribute("data-mpp-local-media-id")?.trim();

    if (localMediaId) {
      ids.add(localMediaId);
    }
  });

  return Array.from(ids);
}

export function hasPendingLocalMedia(html: string) {
  return collectLocalMediaIds(html).length > 0;
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

export function replaceLocalMediaRefs(
  html: string,
  completedRefs: CompletedLocalMediaRef[],
) {
  if (!canUseDomParser() || completedRefs.length === 0) {
    return html;
  }

  const completedByLocalId = new Map(
    completedRefs.map((ref) => [ref.localMediaId, ref]),
  );
  const documentFragment = new DOMParser().parseFromString(html, "text/html");

  documentFragment.querySelectorAll("img").forEach((image) => {
    const localMediaId = image.getAttribute("data-mpp-local-media-id")?.trim();
    const completed = localMediaId
      ? completedByLocalId.get(localMediaId)
      : undefined;

    if (!completed) {
      return;
    }

    image.setAttribute("src", mediaObjectRef(completed.assetId));
    image.removeAttribute("data-mpp-local-media-id");
    image.removeAttribute("data-mpp-upload-status");
    image.setAttribute("data-mpp-media-id", completed.assetId);
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
