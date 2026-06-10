export type PlatformKey = "zhihu" | "xiaohongshu" | "douyin" | "bilibili" | "x";

export type PublishingMode = "remote" | "manual" | "extension";

export type AdapterKey =
  | "ARTICLE_ZHIHU"
  | "NOTE_XIAOHONGSHU"
  | "DYNAMIC_DOUYIN"
  | "DYNAMIC_BILIBILI"
  | "POST_X";

export type ContentKind =
  | "article"
  | "dynamic_post"
  | "image_note"
  | "image_video";

export type TargetFormat = "html" | "markdown" | "text";

export interface PlatformCapability {
  platform: PlatformKey;
  supported_modes: PublishingMode[];
  preferred_mode: PublishingMode;
  adapter_key: AdapterKey;
  inject_url: string;
  inject_urls?: string[];
  content_kinds: ContentKind[];
  target_formats: TargetFormat[];
  requires_review: boolean;
  auto_publish_allowed: boolean;
}
