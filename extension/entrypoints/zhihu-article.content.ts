import { runZhihuArticleAdapter } from "../src/adapters/zhihu-article";
import { registerAdapterRunner } from "../src/adapters/runner";

export default defineContentScript({
  matches: ["https://zhuanlan.zhihu.com/*", "https://www.zhihu.com/*"],
  registration: "runtime",
  main() {
    registerAdapterRunner("ARTICLE_ZHIHU", runZhihuArticleAdapter);
  },
});
