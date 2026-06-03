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
    inject_url: "https://creator.douyin.com/creator-micro/content/upload",
    content_kinds: ["image_video"],
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
] satisfies PlatformCapability[];

export const ADAPTER_SCRIPT_FILES: Record<AdapterKey, ScriptPublicPath> = {
  ARTICLE_ZHIHU: "/content-scripts/zhihu-article.js",
  NOTE_XIAOHONGSHU: "/content-scripts/xiaohongshu-note.js",
  DYNAMIC_DOUYIN: "/content-scripts/douyin-dynamic.js",
  DYNAMIC_BILIBILI: "/content-scripts/bilibili-dynamic.js",
};

export function isSupportedAdapterKey(value: string): value is AdapterKey {
  return Object.hasOwn(ADAPTER_SCRIPT_FILES, value);
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
    const expected = new URL(capability.inject_url);

    return (
      actual.origin === expected.origin &&
      actual.pathname.replace(/\/$/, "") ===
        expected.pathname.replace(/\/$/, "")
    );
  } catch {
    return false;
  }
}
