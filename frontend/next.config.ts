import type { NextConfig } from "next";

const defaultBackendApiBaseUrl = "http://localhost:8080";

function getBackendApiBaseUrl() {
  return (
    process.env.BACKEND_API_BASE_URL?.replace(/\/$/, "") ??
    defaultBackendApiBaseUrl
  );
}

const nextConfig: NextConfig = {
  allowedDevOrigins: ["127.0.0.1"],
  output: "standalone",
  async rewrites() {
    const backendApiBaseUrl = getBackendApiBaseUrl();

    return {
      beforeFiles: [
        {
          destination: `${backendApiBaseUrl}/api/user/dashboard/browser-sessions/:path*`,
          source: "/api/user/dashboard/browser-sessions/:path*",
        },
      ],
    };
  },
};

export default nextConfig;
