import type { Metadata } from "next";

import { useTranslation } from "@/lib/i18n";

export async function generateMetadata({
  params,
}: {
  params: Promise<{ locale: string }>;
}): Promise<Metadata> {
  const { locale } = await params;
  const { t } = await useTranslation(locale, "common");

  return {
    title: t("login.title"),
    robots: {
      index: false,
      follow: false,
    },
  };
}

export default function LoginLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return children;
}
