import path from "node:path";
import { fileURLToPath } from "node:url";
import type { NextConfig } from "next";

const defaultBackendApiBaseUrl = "http://localhost:8080";
const frontendDir = path.dirname(fileURLToPath(import.meta.url));
const workspaceRoot = path.join(frontendDir, "..");

function getBackendApiBaseUrl() {
  return (
    process.env.BACKEND_API_BASE_URL?.replace(/\/$/, "") ??
    defaultBackendApiBaseUrl
  );
}

function isTurbopackFileSystemCacheEnabledForDev() {
  const value = process.env.MPP_FRONTEND_TURBOPACK_FS_CACHE?.toLowerCase();

  if (value === "0" || value === "false" || value === "off") {
    return false;
  }

  return true;
}

function getAllowedDevOrigins() {
  const origins = new Set(["127.0.0.1"]);
  const configuredBaseUrl =
    process.env.FRONTEND_BASE_URL ?? process.env.NEXT_PUBLIC_SITE_URL;

  if (configuredBaseUrl) {
    try {
      const hostname = new URL(configuredBaseUrl).hostname;
      if (hostname && hostname !== "localhost") {
        origins.add(hostname);
      }
    } catch {
      // Ignore malformed local dev URLs; Next will keep the default allowlist.
    }
  }

  return [...origins];
}

const nextConfig: NextConfig = {
  allowedDevOrigins: getAllowedDevOrigins(),
  experimental: {
    turbopackFileSystemCacheForDev: isTurbopackFileSystemCacheEnabledForDev(),
  },
  outputFileTracingRoot: workspaceRoot,
  output: "standalone",
  turbopack: {
    root: workspaceRoot,
  },
  async rewrites() {
    const backendApiBaseUrl = getBackendApiBaseUrl();

    return {
      beforeFiles: [
        {
          destination: `${backendApiBaseUrl}/api/browser-stream/:path*`,
          source: "/api/browser-stream/:path*",
        },
        {
          destination: `${backendApiBaseUrl}/api/user/dashboard/browser-sessions/:path*`,
          source: "/api/user/dashboard/browser-sessions/:path*",
        },
      ],
    };
  },
};

export default nextConfig;
