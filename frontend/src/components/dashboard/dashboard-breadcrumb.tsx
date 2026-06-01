"use client";

import { usePathname } from "next/navigation";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  dashboardRoutes,
  getDashboardPageTitle,
} from "@/lib/dashboard/navigation";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

export function DashboardBreadcrumb() {
  const pathname = usePathname();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");

  const pageTitleKey = getDashboardPageTitle(pathname);

  return (
    <Breadcrumb>
      <BreadcrumbList>
        <BreadcrumbItem className="hidden md:block">
          <BreadcrumbLink href={`/${locale}${dashboardRoutes.overview.url}`}>
            {t(dashboardRoutes.overview.title)}
          </BreadcrumbLink>
        </BreadcrumbItem>
        <BreadcrumbSeparator className="hidden md:block" />
        <BreadcrumbItem>
          <BreadcrumbPage>{t(pageTitleKey)}</BreadcrumbPage>
        </BreadcrumbItem>
      </BreadcrumbList>
    </Breadcrumb>
  );
}
