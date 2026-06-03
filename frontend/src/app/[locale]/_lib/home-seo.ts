import type { Metadata } from "next";

import { useTranslation } from "@/lib/i18n";
import { getIntlLocale } from "@/lib/i18n/settings";
import { siteConfig } from "@/lib/seo";

type HomeMetadataProps = {
  params: Promise<{ locale: string }>;
};

type HomeStructuredDataInput = {
  description: string;
  featureList: string[];
  locale: string;
};

export async function generateHomeMetadata({
  params,
}: HomeMetadataProps): Promise<Metadata> {
  const { locale } = await params;
  const { t } = await useTranslation(locale, "home");

  return {
    title: t("title"),
    description: t("description"),
    alternates: {
      canonical: "/",
    },
  };
}

export function createHomeStructuredData({
  description,
  featureList,
  locale,
}: HomeStructuredDataInput) {
  return JSON.stringify([
    {
      "@context": "https://schema.org",
      "@type": "WebSite",
      name: siteConfig.name,
      alternateName: siteConfig.shortName,
      url: siteConfig.url,
      inLanguage: getIntlLocale(locale),
    },
    {
      "@context": "https://schema.org",
      "@type": "SoftwareApplication",
      name: siteConfig.name,
      alternateName: siteConfig.shortName,
      applicationCategory: "BusinessApplication",
      operatingSystem: "Web",
      url: siteConfig.url,
      description,
      inLanguage: getIntlLocale(locale),
      featureList,
    },
  ]).replace(/</g, "\\u003c");
}
