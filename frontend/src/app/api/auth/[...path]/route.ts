import type { NextRequest } from "next/server";
import {
  type ApiRouteContext,
  proxyApiRequest,
} from "@/app/api/_lib/proxy";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

function proxyRequest(request: NextRequest, context: ApiRouteContext) {
  return proxyApiRequest(request, context, "/api/auth");
}

export const GET = proxyRequest;
export const POST = proxyRequest;
export const PUT = proxyRequest;
export const PATCH = proxyRequest;
export const DELETE = proxyRequest;
export const OPTIONS = proxyRequest;
