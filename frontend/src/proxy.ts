import { NextResponse, NextRequest } from "next/server";
import acceptLanguage from "accept-language";
import { fallbackLng, languages, cookieName } from "./lib/i18n/settings";

acceptLanguage.languages(languages);

export const config = {
  // matcher: '/:lng*'
  matcher: [
    "/((?!api|_next/static|_next/image|assets|icons|tutorial|favicon\\.ico|robots\\.txt|sitemap\\.xml|sw\\.js|site\\.webmanifest).*)",
  ],
};

export function proxy(req: NextRequest) {
  const localeInPath = languages.find((loc) =>
    req.nextUrl.pathname.startsWith(`/${loc}`),
  );
  const localeInReferer = req.headers.has("referer")
    ? languages.find((loc) =>
        new URL(req.headers.get("referer")!).pathname.startsWith(`/${loc}`),
      )
    : undefined;

  let lng: string | null | undefined = localeInReferer;
  if (!lng && req.cookies.has(cookieName))
    lng = acceptLanguage.get(req.cookies.get(cookieName)?.value);
  if (!lng) lng = acceptLanguage.get(req.headers.get("Accept-Language"));
  if (!lng) lng = fallbackLng;

  const projectContentMatch = req.nextUrl.pathname.match(
    new RegExp(`^/(${languages.join("|")})/dashboard/content/([^/]+)$`),
  );
  if (projectContentMatch) {
    const [, locale, projectId] = projectContentMatch;
    const redirectURL = req.nextUrl.clone();
    redirectURL.pathname = `/${locale}/dashboard/content`;
    redirectURL.searchParams.set("projectId", projectId);
    return NextResponse.redirect(redirectURL);
  }

  const shareProjectMatch = req.nextUrl.pathname.match(
    new RegExp(`^(?:/(${languages.join("|")}))?/share/projects/([^/]+)$`),
  );
  if (shareProjectMatch) {
    const [, locale, token] = shareProjectMatch;
    const redirectURL = req.nextUrl.clone();
    redirectURL.pathname = `/${locale ?? lng}/share/projects`;
    redirectURL.searchParams.set("token", token);
    return NextResponse.redirect(redirectURL);
  }

  // Redirect if lng in path is not supported
  if (!localeInPath && !req.nextUrl.pathname.startsWith("/_next")) {
    const response = NextResponse.redirect(
      new URL(`/${lng}${req.nextUrl.pathname}${req.nextUrl.search}`, req.url),
    );
    if (localeInReferer) response.cookies.set(cookieName, localeInReferer);
    return response;
  }

  if (localeInReferer) {
    const response = NextResponse.next();
    response.cookies.set(cookieName, localeInReferer);
    return response;
  }

  return NextResponse.next();
}
