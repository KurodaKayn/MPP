import { runXiaohongshuNoteAdapter } from "../src/adapters/xiaohongshu-note";
import { registerAdapterRunner } from "../src/adapters/runner";

export default defineContentScript({
  matches: ["https://creator.xiaohongshu.com/*"],
  registration: "runtime",
  main() {
    registerAdapterRunner("NOTE_XIAOHONGSHU", runXiaohongshuNoteAdapter);
  },
});
