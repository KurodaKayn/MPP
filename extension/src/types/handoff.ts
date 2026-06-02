import type {
  AdapterKey,
  ContentKind,
  PlatformKey,
  TargetFormat,
} from "./platform";

export const HANDOFF_SCHEMA_VERSION = 1;
export const HANDOFF_TYPE = "mpp.extension_publish_handoff";

export interface ExtensionPublishHandoff {
  schema_version: typeof HANDOFF_SCHEMA_VERSION;
  type: typeof HANDOFF_TYPE;
  execution_id: string;
  expires_at: string;
  project: {
    id: string;
    title: string;
  };
  platforms: ExtensionPublishPlatformHandoff[];
}

export interface ExtensionPublishPlatformHandoff {
  platform: PlatformKey;
  adapter_key: AdapterKey;
  inject_url: string;
  content_kind: ContentKind;
  auto_publish: boolean;
  requires_review: boolean;
  adapted_content: AdaptedContent;
  assets: HandoffAsset[];
  callback?: HandoffCallback;
}

export interface AdaptedContent {
  schema_version: typeof HANDOFF_SCHEMA_VERSION;
  format: TargetFormat;
  markdown?: string;
  html?: string;
  text?: string;
}

export interface HandoffAsset {
  type: "image" | "video";
  source_url: string;
  name: string;
  mime_type: string;
}

export interface HandoffCallback {
  url: string;
  token: string;
}

export interface StoredHandoff {
  handoff: ExtensionPublishHandoff;
  accepted_at: string;
  source_origin: string;
}
