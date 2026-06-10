import { runXPostAdapter } from "../src/adapters/x-post";
import { registerAdapterRunner } from "../src/adapters/runner";

export default defineContentScript({
  matches: ["https://x.com/*", "https://twitter.com/*"],
  registration: "runtime",
  main() {
    registerAdapterRunner("POST_X", runXPostAdapter);
  },
});
