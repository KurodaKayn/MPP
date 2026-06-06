import type { NextRequest } from "next/server";
import { proxyTokenSessionRequest } from "../_lib/token-session-route";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export function POST(request: NextRequest) {
  return proxyTokenSessionRequest(request, "login");
}
