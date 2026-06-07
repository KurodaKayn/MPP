import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "wxt";

export default defineConfig({
  modules: ["@wxt-dev/module-react"],
  webExt: {
    disabled: process.env.WXT_MANUAL_RUNNER === "true",
  },
  manifest: {
    name: "MPP Extension Publisher",
    description:
      "Local browser publishing bridge for Multi-platform Poster drafts.",
    permissions: [
      "activeTab",
      "tabs",
      "scripting",
      "storage",
      "sidePanel",
      "cookies",
    ],
    host_permissions: [
      "https://mpp.example.com/*",
      "http://localhost/*",
      "http://127.0.0.1/*",
      "https://zhuanlan.zhihu.com/*",
      "https://www.zhihu.com/*",
      "https://creator.xiaohongshu.com/*",
      "https://creator.douyin.com/*",
      "https://t.bilibili.com/*",
      "https://member.bilibili.com/*",
    ],
    action: {
      default_title: "MPP Extension Publisher",
    },
    side_panel: {
      default_path: "publish.html",
    },
  },
  vite: () => ({
    plugins: [tailwindcss()],
  }),
});
