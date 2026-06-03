import type { components } from "./generated";

type ContractSchema<Name extends keyof components["schemas"]> =
  components["schemas"][Name];

export type DashboardStats = ContractSchema<"DashboardStats">;
export type AdaptedContent = ContractSchema<"AdaptedContent">;
export type DraftFormat = ContractSchema<"DraftFormat">;
export type PublishPlatform = ContractSchema<"PublishPlatform">;
export type PublicationStatus = ContractSchema<"PublicationStatus">;
export type ProjectStatus = ContractSchema<"ProjectStatus">;
export type PublicationSummary = ContractSchema<"PublicationSummary">;
export type PublicationDetail = ContractSchema<"PublicationDetail">;
export type ProjectPublications = ContractSchema<"ProjectPublications">;
export type PublishResult = ContractSchema<"PublishResult">;

export type PublishProjectOptions = {
  mode?: "manual";
};

export type CreateProjectInput = {
  title: string;
  source_content: string;
  summary?: string;
  cover_image_url?: string;
  platforms: PublishPlatform[];
};

export type UpdateProjectInput = CreateProjectInput;

export type SaveProjectContentInput = Omit<CreateProjectInput, "platforms">;

export type SaveProjectPlatformsInput = {
  platforms: PublishPlatform[];
};

export type GetProjectPublicationsOptions = {
  includeContent?: boolean;
};

export type FetchProjectPublications = (
  projectId: string,
  options?: GetProjectPublicationsOptions,
) => Promise<ProjectPublications>;

export type WaitForProjectPublicationsOptions = {
  intervalMs?: number;
  timeoutMs?: number;
  fetchProjectPublications?: FetchProjectPublications;
  sleep?: (ms: number) => Promise<void>;
};

export type SyncPrepublishInput = {
  platforms: PublishPlatform[];
  actor?: {
    type: "system";
  };
};

export type UpdatePrepublishDraftInput = {
  adapted_content: AdaptedContent;
};

export type AIChatMessage = ContractSchema<"AIChatMessage">;

export type AIEditContentStreamInput = {
  title?: string;
  content: string;
  message: string;
  conversation?: AIChatMessage[];
};

export type AIEditPrepublishStreamInput = {
  title?: string;
  platform: PublishPlatform;
  adapted_content: AdaptedContent;
  message: string;
  conversation?: AIChatMessage[];
};

export type AITextStreamOptions = {
  onChunk?: (chunk: string, accumulated: string) => void;
  signal?: AbortSignal;
};

export type RequirementStatus = ContractSchema<"RequirementStatus">;
export type WechatAccount = ContractSchema<"WechatAccount">;
export type SaveWechatAccountInput = ContractSchema<"SaveWechatAccountInput">;
export type WechatConnectionTestResult =
  ContractSchema<"WechatConnectionTestResult">;
export type XAccount = ContractSchema<"XAccount">;
export type SaveXAccountInput = ContractSchema<"SaveXAccountInput">;
export type XConnectionTestResult = ContractSchema<"XConnectionTestResult">;
export type DouyinAccount = ContractSchema<"DouyinAccount">;
export type ZhihuAccount = ContractSchema<"ZhihuAccount">;

export type BrowserSessionStatus = ContractSchema<"BrowserSessionStatus">;
export type BrowserSession = ContractSchema<"BrowserSession">;
export type StartBrowserSessionResult =
  ContractSchema<"StartBrowserSessionResult">;
export type StartPublishBrowserSessionResult =
  ContractSchema<"StartPublishBrowserSessionResult">;
export type CompleteBrowserSessionResult =
  ContractSchema<"CompleteBrowserSessionResult">;
export type CancelBrowserSessionResult =
  ContractSchema<"CancelBrowserSessionResult">;

export type ProjectListItem = ContractSchema<"ProjectListItem">;
export type ProjectDetail = ContractSchema<"ProjectDetail">;
export type PaginatedProjects = ContractSchema<"PaginationProjects">;
