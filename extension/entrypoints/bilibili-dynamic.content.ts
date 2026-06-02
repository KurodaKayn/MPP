import { runBilibiliDynamicAdapter } from "../src/adapters/bilibili-dynamic";
import { registerAdapterRunner } from "../src/adapters/runner";

export default defineContentScript({
  matches: ["https://t.bilibili.com/*", "https://member.bilibili.com/*"],
  registration: "runtime",
  main() {
    registerAdapterRunner("DYNAMIC_BILIBILI", runBilibiliDynamicAdapter);
  },
});
