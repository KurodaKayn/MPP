import { randomUUID } from "crypto";
import type { NextRequest } from "next/server";
import { authTokenNames, formatBearerToken } from "@/lib/auth/tokens";

const defaultBackendApiBaseUrl = "http://localhost:8080";
const requestIdHeader = "x-request-id";
const traceIdHeader = "x-trace-id";
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

export type ApiRouteContext = {
  params: Promise<{
    path: string[];
  }>;
};

function getBackendApiBaseUrl() {
  return (
    process.env.BACKEND_API_BASE_URL?.replace(/\/$/, "") ??
    defaultBackendApiBaseUrl
  );
}

function buildTargetUrl(
  request: NextRequest,
  targetPrefix: string,
  path: string[],
) {
  const encodedPath = path.map(encodeURIComponent).join("/");
  const targetUrl = new URL(
    `${targetPrefix.replace(/\/$/, "")}/${encodedPath}`,
    getBackendApiBaseUrl(),
  );
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

function ensureTraceHeaders(headers: Headers) {
  const traceId =
    headers.get(requestIdHeader)?.trim() ||
    headers.get(traceIdHeader)?.trim() ||
    randomUUID();
  headers.set(requestIdHeader, traceId);
  headers.set(traceIdHeader, traceId);
  return traceId;
}

function createForwardedHeaders(request: NextRequest) {
  const headers = new Headers(request.headers);
  const forwardedHost =
    request.headers.get("host")?.trim() || request.nextUrl.host;
  const forwardedProto = request.nextUrl.protocol.replace(/:$/, "");

  for (const header of hopByHopHeaders) {
    headers.delete(header);
  }
  headers.delete("host");
  headers.set("x-forwarded-host", forwardedHost);
  headers.set("x-forwarded-proto", forwardedProto || "http");
  applyAuthorizationFromCookie(request, headers);
  ensureTraceHeaders(headers);

  return headers;
}

export async function proxyApiRequest(
  request: NextRequest,
  { params }: ApiRouteContext,
  targetPrefix: string,
) {
  const { path } = await params;
  const method = request.method.toUpperCase();
  const canHaveBody = method !== "GET" && method !== "HEAD";
  const forwardedHeaders = createForwardedHeaders(request);
  const traceId = forwardedHeaders.get(requestIdHeader) ?? randomUUID();
  const response = await fetch(buildTargetUrl(request, targetPrefix, path), {
    body: canHaveBody ? await request.arrayBuffer() : undefined,
    cache: "no-store",
    headers: forwardedHeaders,
    method,
    redirect: "manual",
  });
  const responseHeaders = new Headers(response.headers);

  for (const header of hopByHopHeaders) {
    responseHeaders.delete(header);
  }
  if (!responseHeaders.has(requestIdHeader)) {
    responseHeaders.set(requestIdHeader, traceId);
  }
  if (!responseHeaders.has(traceIdHeader)) {
    responseHeaders.set(traceIdHeader, traceId);
  }

  return new Response(response.body, {
    headers: responseHeaders,
    status: response.status,
    statusText: response.statusText,
  });
}
