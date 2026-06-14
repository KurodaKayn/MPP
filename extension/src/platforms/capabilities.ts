import type { ScriptPublicPath } from "#imports";
import type { AdapterKey, PlatformCapability } from "../types/platform";
import {
  ADAPTER_SCRIPT_FILES,
  PLATFORM_CAPABILITIES,
} from "./capabilities.generated";

const platformCapabilities =
  PLATFORM_CAPABILITIES satisfies readonly PlatformCapability[];

const adapterScriptFiles = ADAPTER_SCRIPT_FILES satisfies Partial<
  Record<AdapterKey, ScriptPublicPath>
>;

export { adapterScriptFiles as ADAPTER_SCRIPT_FILES };
export { platformCapabilities as PLATFORM_CAPABILITIES };

export function isSupportedAdapterKey(value: string): value is AdapterKey {
  return platformCapabilities.some((item) => item.adapter_key === value);
}

export function getAdapterScriptFile(adapterKey: AdapterKey): ScriptPublicPath {
  const scriptFile = adapterScriptFiles[adapterKey];

  if (!scriptFile) {
    throw new Error(`Adapter script is not available for ${adapterKey}.`);
  }

  return scriptFile as ScriptPublicPath;
}

export function getCapabilityByAdapterKey(
  adapterKey: AdapterKey,
): PlatformCapability {
  const capability = platformCapabilities.find(
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
