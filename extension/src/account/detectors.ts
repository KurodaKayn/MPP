export type AccountDetectionStatus = "signed_in" | "signed_out" | "unknown";

export interface AccountDetectionResult {
  status: AccountDetectionStatus;
  reason: string;
}

function hasAnyElement(selectors: string[]): boolean {
  return selectors.some(
    (selector) => document.querySelector(selector) !== null,
  );
}

export function detectZhihuAccount(): AccountDetectionResult {
  if (hasAnyElement(['a[href*="/people/"]', '[class*="Avatar"]'])) {
    return { status: "signed_in", reason: "Zhihu profile UI detected." };
  }

  if (hasAnyElement(['a[href*="/signin"]', 'button[type="submit"]'])) {
    return { status: "signed_out", reason: "Zhihu sign-in UI detected." };
  }

  return { status: "unknown", reason: "Zhihu account state is unclear." };
}

export function detectGenericCreatorAccount(): AccountDetectionResult {
  if (
    hasAnyElement([
      '[contenteditable="true"]',
      "textarea",
      'input[type="file"]',
      '[class*="upload"]',
    ])
  ) {
    return { status: "signed_in", reason: "Creator editor UI detected." };
  }

  if (hasAnyElement(['input[type="password"]', 'button[type="submit"]'])) {
    return { status: "signed_out", reason: "Sign-in UI detected." };
  }

  return { status: "unknown", reason: "Account state is unclear." };
}
