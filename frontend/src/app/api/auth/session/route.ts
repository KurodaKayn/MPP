import { cookies } from "next/headers";
import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import {
  expireAuthCookies,
  getAuthTokenCookie,
  setAuthTokenCookie,
} from "../_lib/session-cookie";
import { formatBearerToken } from "@/lib/auth/tokens";

const appEnvEnv = "APP_ENV";
const defaultBackendApiBaseUrl = "http://localhost:8080";
const mockLoginFlagEnv = "ENABLE_MOCK_LOGIN";
const nodeEnvFallbackEnv = "NODE_ENV";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

function envFlagEnabled(name: string) {
  switch (process.env[name]?.trim().toLowerCase()) {
    case "1":
    case "true":
    case "yes":
    case "y":
    case "on":
      return true;
    default:
      return false;
  }
}

function isLocalEnvironment(value: string | undefined) {
  switch (value?.trim().toLowerCase()) {
    case "local":
    case "dev":
    case "development":
      return true;
    default:
      return false;
  }
}

function mockLoginEnabled() {
  const localEnv =
    isLocalEnvironment(process.env[appEnvEnv]) ||
    isLocalEnvironment(process.env[nodeEnvFallbackEnv]);
  return envFlagEnabled(mockLoginFlagEnv) && localEnv;
}

function getBackendApiBaseUrl() {
  return (
    process.env.BACKEND_API_BASE_URL?.replace(/\/$/, "") ??
    defaultBackendApiBaseUrl
  );
}

function createSessionResponse(
  authenticated: boolean,
  options: { clearCookies?: boolean } = {},
) {
  const response = NextResponse.json({
    authenticated,
    loginMethods: {
      mock: mockLoginEnabled(),
      token: true,
    },
  });

  if (options.clearCookies) {
    expireAuthCookies(response);
  }

  return response;
}

async function verifyAuthCookie(token: string) {
  const response = await fetch(
    new URL("/api/user/dashboard/stats", getBackendApiBaseUrl()),
    {
      cache: "no-store",
      headers: {
        Accept: "application/json",
        Authorization: formatBearerToken(token),
      },
    },
  );

  return response.ok;
}

export async function GET() {
  const cookieStore = await cookies();
  const token = getAuthTokenCookie(cookieStore);

  if (!token) {
    return createSessionResponse(false);
  }

  const authenticated = await verifyAuthCookie(token).catch(() => false);
  return createSessionResponse(authenticated, {
    clearCookies: !authenticated,
  });
}

export async function POST(request: NextRequest) {
  const body = (await request.json().catch(() => ({}))) as {
    token?: unknown;
  };
  const token = typeof body.token === "string" ? body.token.trim() : "";

  if (!token) {
    return NextResponse.json(
      {
        error: {
          code: "missing_token",
          message: "Access token is required",
        },
      },
      { status: 400 },
    );
  }

  const authenticated = await verifyAuthCookie(token).catch(() => false);
  if (!authenticated) {
    const response = NextResponse.json(
      {
        error: {
          code: "invalid_token",
          message: "Invalid or expired access token",
        },
      },
      { status: 401 },
    );
    expireAuthCookies(response);
    return response;
  }

  const response = createSessionResponse(true);
  setAuthTokenCookie(response, token, request);
  return response;
}

export function DELETE() {
  const response = NextResponse.json({ ok: true });

  expireAuthCookies(response);

  return response;
}
