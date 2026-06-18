import type { NextRequest } from "next/server";
import { type ApiRouteContext, proxyApiRequest } from "@/app/api/_lib/proxy";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

function proxyRequest(request: NextRequest, context: ApiRouteContext) {
  return proxyApiRequest(request, context, "/api/auth");
}

export const POST = proxyRequest;
