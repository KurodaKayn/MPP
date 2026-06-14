#!/usr/bin/env node
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type DraftFormat = "html" | "markdown" | "text";

interface PlatformCapabilityContract {
  schema_version: 1;
  platforms: PlatformCapability[];
}

interface DraftCapability {
  profile: string;
  schema_version: number;
  format: DraftFormat;
}

interface PublishingCapability {
  modes: string[];
  preferred_mode: string;
  auto_publish_allowed: boolean;
  uses_stored_browser_cookies: boolean;
}

interface AccountCapability {
  mode: string;
}

interface FrontendCapability {
  tab: boolean;
  label_key: string;
  default_label: string;
  icon: string;
}

interface ExtensionCapability {
  backend_handoff: boolean;
  supported_modes: string[];
  preferred_mode: string;
  adapter_key: string;
  inject_url: string;
  inject_urls?: string[];
  content_kinds: string[];
  handoff_content_kind?: string;
  target_formats: DraftFormat[];
  requires_review: boolean;
  auto_publish_allowed: boolean;
  script_file: string;
}

interface PlatformCapability {
  key: string;
  display_name: string;
  project: {
    selectable: boolean;
  };
  draft: DraftCapability | null;
  publishing: PublishingCapability;
  account: AccountCapability;
  frontend: FrontendCapability;
  extension: ExtensionCapability | null;
}

type PlatformWithDraft = PlatformCapability & { draft: DraftCapability };
type PlatformWithExtension = PlatformCapability & {
  extension: ExtensionCapability;
};

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const contractPath = resolve(root, "contracts/platform-capabilities.json");
const contract = JSON.parse(
  readFileSync(contractPath, "utf8"),
) as PlatformCapabilityContract;

validateContract(contract);

const platforms = contract.platforms;
const projectPlatforms = platforms.filter(
  (platform) => platform.project.selectable,
);
const extensionPlatforms = platforms.filter(hasExtension);
const extensionHandoffPlatforms = extensionPlatforms.filter(
  (platform) => platform.extension.backend_handoff,
);
const draftPlatforms = platforms.filter(hasDraft);
const storedCookiePlatforms = platforms.filter(
  (platform) => platform.publishing.uses_stored_browser_cookies,
);

updateContentContract(projectPlatforms);
writeBackendCapabilities();
writeRustDraftProfiles();
writeFrontendCapabilities();
writeExtensionCapabilities();

function validateContract(value: PlatformCapabilityContract): void {
  if (value.schema_version !== 1) {
    throw new Error("platform capability schema_version must be 1");
  }
  if (!Array.isArray(value.platforms) || value.platforms.length === 0) {
    throw new Error("platform capability contract must define platforms");
  }
  const keys = new Set();
  const adapterKeys = new Set();
  for (const platform of value.platforms) {
    requireString(platform.key, "platform key");
    if (keys.has(platform.key)) {
      throw new Error(`duplicate platform key: ${platform.key}`);
    }
    keys.add(platform.key);
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(platform.key)) {
      throw new Error(`invalid platform key: ${platform.key}`);
    }
    if (!platform.project || typeof platform.project.selectable !== "boolean") {
      throw new Error(`${platform.key} must define project.selectable`);
    }
    if (platform.project.selectable && !platform.draft) {
      throw new Error(
        `${platform.key} is project selectable but has no draft profile`,
      );
    }
    if (platform.extension) {
      requireString(
        platform.extension.adapter_key,
        `${platform.key} adapter_key`,
      );
      requireString(
        platform.extension.inject_url,
        `${platform.key} inject_url`,
      );
      requireString(
        platform.extension.script_file,
        `${platform.key} script_file`,
      );
      if (adapterKeys.has(platform.extension.adapter_key)) {
        throw new Error(
          `duplicate adapter key: ${platform.extension.adapter_key}`,
        );
      }
      adapterKeys.add(platform.extension.adapter_key);
      if (platform.extension.backend_handoff && !platform.project.selectable) {
        throw new Error(
          `${platform.key} backend handoff requires project.selectable`,
        );
      }
    }
  }
}

function hasDraft(platform: PlatformCapability): platform is PlatformWithDraft {
  return platform.draft !== null;
}

function hasExtension(
  platform: PlatformCapability,
): platform is PlatformWithExtension {
  return platform.extension !== null;
}

function requireString(value: unknown, label: string): void {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`${label} must be a non-empty string`);
  }
}

function updateContentContract(
  selectablePlatforms: PlatformCapability[],
): void {
  const contentPath = resolve(root, "contracts/components/content.yaml");
  const original = readFileSync(contentPath, "utf8");
  const replacement =
    "$1" +
    selectablePlatforms
      .map((platform) => `        - ${platform.key}\n`)
      .join("");
  const publishPlatformPattern =
    /(    PublishPlatform:\n      type: string\n      enum:\n)(?:        - .+\n)+/;
  if (!publishPlatformPattern.test(original)) {
    throw new Error(
      "failed to update PublishPlatform enum in contracts/components/content.yaml",
    );
  }
  const updated = original.replace(publishPlatformPattern, replacement);
  writeFileSync(contentPath, updated);
}

function writeBackendCapabilities(): void {
  const out = `// Code generated by contracts/generate-platform-capabilities.ts; DO NOT EDIT.

package platformcapabilities

type DraftCapability struct {
\tProfile       string
\tSchemaVersion int
\tFormat        string
}

type PublishingCapability struct {
\tModes                    []string
\tPreferredMode            string
\tAutoPublishAllowed       bool
\tUsesStoredBrowserCookies bool
}

type FrontendCapability struct {
\tTab          bool
\tLabelKey     string
\tDefaultLabel string
\tIcon         string
}

type ExtensionCapability struct {
\tBackendHandoff     bool
\tSupportedModes     []string
\tPreferredMode      string
\tAdapterKey         string
\tInjectURL          string
\tInjectURLs         []string
\tContentKinds       []string
\tHandoffContentKind string
\tTargetFormats      []string
\tRequiresReview     bool
\tAutoPublishAllowed bool
\tScriptFile         string
}

type AccountCapability struct {
\tMode string
}

type PlatformCapability struct {
\tKey           string
\tDisplayName   string
\tProject       bool
\tDraft         *DraftCapability
\tPublishing    PublishingCapability
\tAccount       AccountCapability
\tFrontend      FrontendCapability
\tExtension     *ExtensionCapability
}

type ExtensionHandoffConfig struct {
\tAdapterKey      string
\tInjectURL       string
\tContentKind     string
\tRequiresReview  bool
\tAutoPublish     bool
}

const SchemaVersion = ${contract.schema_version}

var Capabilities = []PlatformCapability{
${platforms.map(goPlatformCapability).join("")}}

var ProjectPlatformKeys = []string{${projectPlatforms.map((platform) => goQuote(platform.key)).join(", ")}}

var ExtensionHandoffPlatformKeys = []string{${extensionHandoffPlatforms.map((platform) => goQuote(platform.key)).join(", ")}}

var storedBrowserCookiePlatformSet = map[string]struct{}{
${storedCookiePlatforms.map((platform) => `\t${goQuote(platform.key)}: {},\n`).join("")}}

var extensionHandoffConfigs = map[string]ExtensionHandoffConfig{
${extensionHandoffPlatforms.map(goExtensionHandoffConfig).join("")}}

func ProjectPlatformSet() map[string]struct{} {
\treturn stringSet(ProjectPlatformKeys)
}

func ExtensionHandoffConfigFor(platform string) (ExtensionHandoffConfig, bool) {
\tconfig, ok := extensionHandoffConfigs[platform]
\treturn config, ok
}

func UsesStoredBrowserCookies(platform string) bool {
\t_, ok := storedBrowserCookiePlatformSet[platform]
\treturn ok
}

func stringSet(values []string) map[string]struct{} {
\tset := make(map[string]struct{}, len(values))
\tfor _, value := range values {
\t\tset[value] = struct{}{}
\t}
\treturn set
}
`;
  writeFileSync(
    generatedFile(
      "backend/internal/platformcapabilities/capabilities.generated.go",
    ),
    out,
  );
}

function goPlatformCapability(platform: PlatformCapability): string {
  return `\t{
\t\tKey: ${goQuote(platform.key)},
\t\tDisplayName: ${goQuote(platform.display_name)},
\t\tProject: ${platform.project.selectable},
\t\tDraft: ${goDraftCapability(platform.draft)},
\t\tPublishing: PublishingCapability{
\t\t\tModes: ${goStringSlice(platform.publishing.modes)},
\t\t\tPreferredMode: ${goQuote(platform.publishing.preferred_mode)},
\t\t\tAutoPublishAllowed: ${platform.publishing.auto_publish_allowed},
\t\t\tUsesStoredBrowserCookies: ${platform.publishing.uses_stored_browser_cookies},
\t\t},
\t\tAccount: AccountCapability{Mode: ${goQuote(platform.account.mode)}},
\t\tFrontend: FrontendCapability{
\t\t\tTab: ${platform.frontend.tab},
\t\t\tLabelKey: ${goQuote(platform.frontend.label_key)},
\t\t\tDefaultLabel: ${goQuote(platform.frontend.default_label)},
\t\t\tIcon: ${goQuote(platform.frontend.icon)},
\t\t},
\t\tExtension: ${goExtensionCapability(platform.extension)},
\t},
`;
}

function goDraftCapability(draft: DraftCapability | null): string {
  if (!draft) {
    return "nil";
  }
  return `&DraftCapability{Profile: ${goQuote(draft.profile)}, SchemaVersion: ${draft.schema_version}, Format: ${goQuote(draft.format)}}`;
}

function goExtensionCapability(extension: ExtensionCapability | null): string {
  if (!extension) {
    return "nil";
  }
  return `&ExtensionCapability{
\t\t\tBackendHandoff: ${extension.backend_handoff},
\t\t\tSupportedModes: ${goStringSlice(extension.supported_modes)},
\t\t\tPreferredMode: ${goQuote(extension.preferred_mode)},
\t\t\tAdapterKey: ${goQuote(extension.adapter_key)},
\t\t\tInjectURL: ${goQuote(extension.inject_url)},
\t\t\tInjectURLs: ${goStringSlice(extension.inject_urls ?? [])},
\t\t\tContentKinds: ${goStringSlice(extension.content_kinds)},
\t\t\tHandoffContentKind: ${goQuote(extension.handoff_content_kind ?? extension.content_kinds[0])},
\t\t\tTargetFormats: ${goStringSlice(extension.target_formats)},
\t\t\tRequiresReview: ${extension.requires_review},
\t\t\tAutoPublishAllowed: ${extension.auto_publish_allowed},
\t\t\tScriptFile: ${goQuote(extension.script_file)},
\t\t}`;
}

function goExtensionHandoffConfig(platform: PlatformWithExtension): string {
  const extension = platform.extension;
  return `\t${goQuote(platform.key)}: {
\t\tAdapterKey: ${goQuote(extension.adapter_key)},
\t\tInjectURL: ${goQuote(extension.inject_url)},
\t\tContentKind: ${goQuote(extension.handoff_content_kind ?? extension.content_kinds[0])},
\t\tRequiresReview: ${extension.requires_review},
\t\tAutoPublish: ${extension.auto_publish_allowed},
\t},
`;
}

function goStringSlice(values?: readonly string[] | null): string {
  if (!values || values.length === 0) {
    return "nil";
  }
  return `[]string{${values.map(goQuote).join(", ")}}`;
}

function goQuote(value: string): string {
  return JSON.stringify(value);
}

function writeRustDraftProfiles(): void {
  const out = `// Code generated by contracts/generate-platform-capabilities.ts; DO NOT EDIT.

use super::{DraftFormat, DraftProfile};

pub(super) const SUPPORTED_DRAFT_PROFILES: &[DraftProfile] = &[
${draftPlatforms.map(rustDraftProfile).join("")}];
`;
  writeFileSync(
    generatedFile(
      "content-pipeline-service/crates/content-pipeline-core/src/drafts/profiles_generated.rs",
    ),
    out,
  );
}

function rustDraftProfile(platform: PlatformWithDraft): string {
  return `    DraftProfile {
        platform: "${platform.key}",
        profile: "${platform.draft.profile}",
        schema_version: ${platform.draft.schema_version},
        format: DraftFormat::${rustDraftFormat(platform.draft.format)},
    },
`;
}

function rustDraftFormat(format: DraftFormat): string {
  switch (format) {
    case "html":
      return "Html";
    case "markdown":
      return "Markdown";
    case "text":
      return "Text";
    default:
      throw new Error(`unsupported draft format: ${format}`);
  }
}

function writeFrontendCapabilities(): void {
  const tabs = projectPlatforms
    .filter(hasDraft)
    .filter((platform) => platform.frontend.tab)
    .map((platform) => ({
      value: platform.key,
      label: platform.frontend.label_key,
      defaultLabel: platform.frontend.default_label,
      icon: platform.frontend.icon,
      autoPublishAllowed: platform.publishing.auto_publish_allowed,
      preferredPublishMode: platform.publishing.preferred_mode,
      draftFormat: platform.draft.format,
    }));
  const out = `// Code generated by contracts/generate-platform-capabilities.ts; DO NOT EDIT.

export const PLATFORM_CAPABILITY_SCHEMA_VERSION = ${contract.schema_version} as const;

export const PLATFORM_TABS = ${tsConst(tabs)} as const;
`;
  writeFileSync(
    generatedFile(
      "frontend/src/lib/content/platform-capabilities.generated.ts",
    ),
    out,
  );
}

function writeExtensionCapabilities(): void {
  const capabilities = extensionPlatforms.map((platform) => {
    const extension = platform.extension;
    return {
      platform: platform.key,
      supported_modes: extension.supported_modes,
      preferred_mode: extension.preferred_mode,
      adapter_key: extension.adapter_key,
      inject_url: extension.inject_url,
      ...(extension.inject_urls ? { inject_urls: extension.inject_urls } : {}),
      content_kinds: extension.content_kinds,
      target_formats: extension.target_formats,
      requires_review: extension.requires_review,
      auto_publish_allowed: extension.auto_publish_allowed,
    };
  });
  const scriptFiles = Object.fromEntries(
    extensionPlatforms.map((platform) => [
      platform.extension.adapter_key,
      platform.extension.script_file,
    ]),
  );
  const out = `// Code generated by contracts/generate-platform-capabilities.ts; DO NOT EDIT.

export const PLATFORM_CAPABILITY_SCHEMA_VERSION = ${contract.schema_version} as const;

export const PLATFORM_CAPABILITIES = ${tsConst(capabilities)} as const;

export const ADAPTER_SCRIPT_FILES = ${tsConst(scriptFiles)} as const;
`;
  writeFileSync(
    generatedFile("extension/src/platforms/capabilities.generated.ts"),
    out,
  );
}

function tsConst(value: unknown): string {
  return JSON.stringify(value, null, 2).replace(/"([^"]+)":/g, "$1:");
}

function generatedFile(path: string): string {
  const absolutePath = resolve(root, path);
  mkdirSync(dirname(absolutePath), { recursive: true });
  return absolutePath;
}
