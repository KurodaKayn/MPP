import type { NextRequest, NextResponse } from "next/server";
import { authTokenNames, primaryAuthTokenName } from "@/lib/auth/tokens";

const sessionMaxAgeSeconds = 60 * 60 * 24 * 3;

type CookieStore = {
  get: (name: string) => { value?: string } | undefined;
};

function isSecureRequest(request: NextRequest | undefined) {
  if (process.env.NODE_ENV === "production") {
    return true;
  }

  const forwardedProto = request?.headers
    .get("x-forwarded-proto")
    ?.split(",")[0]
    ?.trim()
    .toLowerCase();

  return request?.nextUrl.protocol === "https:" || forwardedProto === "https";
}

export function getAuthTokenCookie(cookieStore: CookieStore) {
  return authTokenNames
    .map((name) => cookieStore.get(name)?.value)
    .find(Boolean);
}

export function setAuthTokenCookie(
  response: NextResponse,
  token: string,
  request?: NextRequest,
) {
  response.cookies.set(primaryAuthTokenName, token, {
    httpOnly: true,
    maxAge: sessionMaxAgeSeconds,
    path: "/",
    sameSite: "lax",
    secure: isSecureRequest(request),
  });

  for (const name of authTokenNames) {
    if (name === primaryAuthTokenName) {
      continue;
    }

    response.cookies.set(name, "", {
      httpOnly: true,
      maxAge: 0,
      path: "/",
      sameSite: "lax",
      secure: isSecureRequest(request),
    });
  }
}

export function expireAuthCookies(response: NextResponse) {
  for (const name of authTokenNames) {
    response.cookies.set(name, "", {
      httpOnly: true,
      maxAge: 0,
      path: "/",
      sameSite: "lax",
    });
  }
}
