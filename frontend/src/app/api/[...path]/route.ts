import type { NextRequest } from "next/server";
import { authTokenNames, formatBearerToken } from "../../../lib/auth/tokens";

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

type ApiRouteContext = {
  params: Promise<{
    path: string[];
  }>;
};

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

function getBackendApiBaseUrl() {
  return (
    process.env.BACKEND_API_BASE_URL?.replace(/\/$/, "") ??
    defaultBackendApiBaseUrl
  );
}

function buildTargetUrl(request: NextRequest, path: string[]) {
  const encodedPath = path.map(encodeURIComponent).join("/");
  const targetUrl = new URL(`/api/${encodedPath}`, getBackendApiBaseUrl());
  targetUrl.search = request.nextUrl.search;
  return targetUrl;
}

function applyAuthorizationFromCookie(request: NextRequest, headers: Headers) {
  if (headers.has("authorization")) {
    return;
  }

  for (const name of authTokenNames) {
    const token = request.cookies.get(name)?.value;
    if (token) {
      headers.set("authorization", formatBearerToken(token));
      return;
    }
  }
}

function createForwardedHeaders(request: NextRequest) {
  const headers = new Headers(request.headers);

  for (const header of hopByHopHeaders) {
    headers.delete(header);
  }
  headers.delete("host");
  applyAuthorizationFromCookie(request, headers);

  return headers;
}

async function proxyRequest(request: NextRequest, { params }: ApiRouteContext) {
  const { path } = await params;
  const method = request.method.toUpperCase();
  const canHaveBody = method !== "GET" && method !== "HEAD";
  const response = await fetch(buildTargetUrl(request, path), {
    body: canHaveBody ? await request.arrayBuffer() : undefined,
    cache: "no-store",
    headers: createForwardedHeaders(request),
    method,
    redirect: "manual",
  });
  const responseHeaders = new Headers(response.headers);

  for (const header of hopByHopHeaders) {
    responseHeaders.delete(header);
  }

  return new Response(response.body, {
    headers: responseHeaders,
    status: response.status,
    statusText: response.statusText,
  });
}

export const GET = proxyRequest;
export const POST = proxyRequest;
export const PUT = proxyRequest;
export const PATCH = proxyRequest;
export const DELETE = proxyRequest;
export const OPTIONS = proxyRequest;
