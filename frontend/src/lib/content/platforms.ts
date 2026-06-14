import { PLATFORM_TABS } from "./platform-capabilities.generated";

export { PLATFORM_TABS };

export type PlatformTab = (typeof PLATFORM_TABS)[number];

export const AUTO_PUBLISH_PLATFORM_TABS = PLATFORM_TABS.filter(
  (platform) => platform.autoPublishAllowed,
);

export function getPlatformDefaultLabel(platform: string) {
  return (
    PLATFORM_TABS.find((item) => item.value === platform)?.defaultLabel ??
    platform
  );
}
