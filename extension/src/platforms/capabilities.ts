import type { ScriptPublicPath } from "#imports";
import type { AdapterKey, PlatformCapability } from "../types/platform";

export const PLATFORM_CAPABILITIES = [
  {
    platform: "zhihu",
    supported_modes: ["extension", "remote"],
    preferred_mode: "extension",
    adapter_key: "ARTICLE_ZHIHU",
    inject_url: "https://zhuanlan.zhihu.com/write",
    content_kinds: ["article"],
    target_formats: ["markdown", "html"],
    requires_review: true,
    auto_publish_allowed: false,
  },
  {
    platform: "xiaohongshu",
    supported_modes: ["extension"],
    preferred_mode: "extension",
    adapter_key: "NOTE_XIAOHONGSHU",
    inject_url: "https://creator.xiaohongshu.com/publish/publish",
    content_kinds: ["image_note"],
    target_formats: ["text"],
    requires_review: true,
    auto_publish_allowed: false,
  },
  {
    platform: "douyin",
    supported_modes: ["extension"],
    preferred_mode: "extension",
    adapter_key: "DYNAMIC_DOUYIN",
    inject_url:
      "https://creator.douyin.com/creator-micro/content/upload?default-tab=5",
    inject_urls: [
      "https://creator.douyin.com/creator-micro/content/upload",
      "https://creator.douyin.com/creator-micro/content/post/article",
    ],
    content_kinds: ["article", "image_video"],
    target_formats: ["text"],
    requires_review: true,
    auto_publish_allowed: false,
  },
  {
    platform: "bilibili",
    supported_modes: ["extension"],
    preferred_mode: "extension",
    adapter_key: "DYNAMIC_BILIBILI",
    inject_url: "https://t.bilibili.com",
    content_kinds: ["dynamic_post"],
    target_formats: ["text"],
    requires_review: true,
    auto_publish_allowed: false,
  },
  {
    platform: "x",
    supported_modes: ["extension"],
    preferred_mode: "extension",
    adapter_key: "POST_X",
    inject_url: "https://x.com/compose/post",
    content_kinds: ["dynamic_post"],
    target_formats: ["text"],
    requires_review: true,
    auto_publish_allowed: false,
  },
] satisfies PlatformCapability[];

export const ADAPTER_SCRIPT_FILES: Partial<
  Record<AdapterKey, ScriptPublicPath>
> = {
  ARTICLE_ZHIHU: "/content-scripts/zhihu-article.js",
  NOTE_XIAOHONGSHU: "/content-scripts/xiaohongshu-note.js",
  DYNAMIC_DOUYIN: "/content-scripts/douyin-dynamic.js",
  DYNAMIC_BILIBILI: "/content-scripts/bilibili-dynamic.js",
} satisfies Partial<Record<AdapterKey, ScriptPublicPath>>;

export function isSupportedAdapterKey(value: string): value is AdapterKey {
  return PLATFORM_CAPABILITIES.some((item) => item.adapter_key === value);
}

export function getAdapterScriptFile(adapterKey: AdapterKey): ScriptPublicPath {
  const scriptFile = ADAPTER_SCRIPT_FILES[adapterKey];

  if (!scriptFile) {
    throw new Error(`Adapter script is not available for ${adapterKey}.`);
  }

  return scriptFile;
}

export function getCapabilityByAdapterKey(
  adapterKey: AdapterKey,
): PlatformCapability {
  const capability = PLATFORM_CAPABILITIES.find(
    (item) => item.adapter_key === adapterKey,
  );

  if (!capability) {
    throw new Error(`Unsupported adapter key: ${adapterKey}`);
  }

  return capability;
}

export function isCapabilityInjectUrl(
  adapterKey: AdapterKey,
  value: string,
): boolean {
  const capability = getCapabilityByAdapterKey(adapterKey);

  try {
    const actual = new URL(value);
    const expectedUrls = capability.inject_urls ?? [capability.inject_url];

    return expectedUrls.some((expectedValue) => {
      const expected = new URL(expectedValue);

      return (
        actual.origin === expected.origin &&
        actual.pathname.replace(/\/$/, "") ===
          expected.pathname.replace(/\/$/, "")
      );
    });
  } catch {
    return false;
  }
}
