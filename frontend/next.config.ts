import type { NextConfig } from "next";

const defaultBackendApiBaseUrl = "http://localhost:8080";

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

const nextConfig: NextConfig = {
  allowedDevOrigins: ["127.0.0.1"],
  experimental: {
    turbopackFileSystemCacheForDev: isTurbopackFileSystemCacheEnabledForDev(),
  },
  output: "standalone",
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
