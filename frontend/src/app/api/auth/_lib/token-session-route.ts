import type { NextRequest } from "next/server";
import { NextResponse } from "next/server";
import { setAuthTokenCookie } from "./session-cookie";

const defaultBackendApiBaseUrl = "http://localhost:8080";
const hopByHopHeaders = [
  "connection",
  "content-length",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
];

type AuthTokenResponse = Record<string, unknown> & {
  token?: unknown;
};

function getBackendApiBaseUrl() {
  return (
    process.env.BACKEND_API_BASE_URL?.replace(/\/$/, "") ??
    defaultBackendApiBaseUrl
  );
}

function createForwardedHeaders(request: NextRequest) {
  const headers = new Headers(request.headers);
  for (const header of hopByHopHeaders) {
    headers.delete(header);
  }
  headers.delete("host");
  return headers;
}

function createResponseHeaders(source: Headers) {
  const headers = new Headers(source);
  for (const header of hopByHopHeaders) {
    headers.delete(header);
  }
  headers.delete("set-cookie");
  return headers;
}

export async function proxyTokenSessionRequest(
  request: NextRequest,
  authPath: "login" | "register",
) {
  const targetUrl = new URL(`/api/auth/${authPath}`, getBackendApiBaseUrl());
  targetUrl.search = request.nextUrl.search;

  const backendResponse = await fetch(targetUrl, {
    body: await request.arrayBuffer(),
    cache: "no-store",
    headers: createForwardedHeaders(request),
    method: request.method,
    redirect: "manual",
  });
  const responseHeaders = createResponseHeaders(backendResponse.headers);
  const body = (await backendResponse.json().catch(() => ({}))) as
    | AuthTokenResponse
    | undefined;

  if (!backendResponse.ok) {
    return NextResponse.json(body ?? {}, {
      headers: responseHeaders,
      status: backendResponse.status,
      statusText: backendResponse.statusText,
    });
  }

  const token = typeof body?.token === "string" ? body.token.trim() : "";
  if (!token) {
    return NextResponse.json(
      {
        error: {
          code: "missing_token",
          message: "Authentication service did not return a token",
        },
      },
      { status: 502 },
    );
  }

  const safeBody = { ...body, authenticated: true };
  delete safeBody.token;

  const response = NextResponse.json(safeBody, {
    headers: responseHeaders,
    status: backendResponse.status,
    statusText: backendResponse.statusText,
  });
  setAuthTokenCookie(response, token, request);
  return response;
}
