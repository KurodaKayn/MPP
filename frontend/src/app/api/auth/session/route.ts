import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { authTokenNames } from "../../../../lib/auth/tokens";

const appEnvEnv = "APP_ENV";
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

export async function GET() {
  const cookieStore = await cookies();
  const authenticated = authTokenNames.some((name) =>
    Boolean(cookieStore.get(name)?.value),
  );

  return NextResponse.json({
    authenticated,
    loginMethods: {
      mock: mockLoginEnabled(),
      token: true,
    },
  });
}

export function DELETE() {
  const response = NextResponse.json({ ok: true });

  for (const name of authTokenNames) {
    response.cookies.set(name, "", {
      maxAge: 0,
      path: "/",
      sameSite: "lax",
    });
  }

  return response;
}
