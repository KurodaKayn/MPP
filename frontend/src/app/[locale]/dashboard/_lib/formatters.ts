import { getIntlLocale } from "@/lib/i18n/settings";

export function formatDashboardNumber(value: number, locale: string) {
  return new Intl.NumberFormat(getIntlLocale(locale)).format(value);
}

export function formatDashboardDate(value: string, locale: string) {
  return new Intl.DateTimeFormat(getIntlLocale(locale), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function formatOptionalDashboardDate(
  value: string | undefined,
  locale: string,
  fallback: string,
) {
  return value ? formatDashboardDate(value, locale) : fallback;
}
