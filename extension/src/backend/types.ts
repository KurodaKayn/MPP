import type { ExtensionPublishHandoff } from "../types/handoff";
import type { AdapterKey, ContentKind, PlatformKey } from "../types/platform";

export interface ExtensionSessionUser {
  id: string;
  username: string;
}

export interface ExtensionSessionResponse {
  authenticated: boolean;
  user: ExtensionSessionUser;
}

export interface ExtensionPrepublishPlatform {
  publication_id: string;
  platform: PlatformKey;
  adapter_key: AdapterKey;
  content_kind: ContentKind;
  status: string;
  enabled: boolean;
  preview: string;
}

export interface ExtensionPrepublishItem {
  project_id: string;
  title: string;
  status: string;
  updated_at: string;
  platforms: ExtensionPrepublishPlatform[];
}

export interface ExtensionPrepublishResponse {
  items: ExtensionPrepublishItem[];
}

export interface CreateExtensionHandoffRequest {
  project_id: string;
  platforms: PlatformKey[];
}

export type BackendExtensionPublishHandoff = ExtensionPublishHandoff;

export interface BackendErrorPayload {
  error?: {
    code?: string;
    message?: string;
  };
}
