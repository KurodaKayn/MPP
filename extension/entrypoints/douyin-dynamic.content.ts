import { runDouyinDynamicAdapter } from "../src/adapters/douyin-dynamic";
import { registerAdapterRunner } from "../src/adapters/runner";

export default defineContentScript({
  matches: ["https://creator.douyin.com/*"],
  registration: "runtime",
  main() {
    registerAdapterRunner("DYNAMIC_DOUYIN", runDouyinDynamicAdapter);
  },
});
