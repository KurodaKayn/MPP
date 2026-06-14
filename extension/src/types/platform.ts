import type {
  ADAPTER_SCRIPT_FILES,
  PLATFORM_CAPABILITIES,
} from "../platforms/capabilities.generated";

export type PlatformKey = (typeof PLATFORM_CAPABILITIES)[number]["platform"];

export type PublishingMode = "remote" | "manual" | "extension";

export type AdapterKey = keyof typeof ADAPTER_SCRIPT_FILES;

export type ContentKind =
  (typeof PLATFORM_CAPABILITIES)[number]["content_kinds"][number];

export type TargetFormat =
  (typeof PLATFORM_CAPABILITIES)[number]["target_formats"][number];

export interface PlatformCapability {
  platform: PlatformKey;
  supported_modes: readonly PublishingMode[];
  preferred_mode: PublishingMode;
  adapter_key: AdapterKey;
  inject_url: string;
  inject_urls?: readonly string[];
  content_kinds: readonly ContentKind[];
  target_formats: readonly TargetFormat[];
  requires_review: boolean;
  auto_publish_allowed: boolean;
}
