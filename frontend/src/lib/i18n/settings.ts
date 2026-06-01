export const fallbackLng = "zh";
export const languages = [fallbackLng, "en"];
export const defaultNS = "common";
export const cookieName = "i18next";

export function getOptions(lng = fallbackLng, ns = defaultNS) {
  return {
    // debug: true,
    supportedLngs: languages,
    fallbackLng,
    lng,
    fallbackNS: defaultNS,
    defaultNS,
    ns,
  };
}

export function getIntlLocale(lng: string): string {
  const map: Record<string, string> = {
    zh: "zh-CN",
    en: "en-US",
  };
  return map[lng] || "en-US";
}
