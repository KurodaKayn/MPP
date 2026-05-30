# Platform Publishing Method Classification

This document is used to determine whether the multi-platform publishing module should prioritize integrating the official API or can only manipulate the DOM through a browser extension to simulate manual publishing. Conclusions are not permanent: each platform's open capabilities, review thresholds, and interface permissions are subject to change. Re-verify official documentation and account qualifications before adding or refactoring platform adapters.

## Platforms Supporting API Publishing

| Platform                               | API Publishing Capability        | Remarks                                                                                                                                            |
| -------------------------------------- | -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| WeChat Official Account                | Drafts, Material Upload, Publish | Priority should be given to the official API. Requires AppID/AppSecret, account permissions, and IP whitelist.                                     |
| Bilibili Video/Article                 | Video/Article Submissions        | Requires Open Platform registration, identity verification, app review, and creator authorization. Bilibili Dynamic is not API-enabled by default. |
| X (Twitter)                            | Tweets, Media, Replies           | Requires developer account, write permissions, and OAuth.                                                                                          |
| LinkedIn                               | Personal/Organization Updates    | Requires OAuth scope; organization pages require admin authorization.                                                                              |
| Facebook Page                          | Posts, Images, Videos            | Requires Meta App, Page access token, and permission review.                                                                                       |
| Instagram Business/Creator             | Images, Videos, Reels            | Requires Graph API; account must be Business/Creator and linked to a Facebook Page.                                                                |
| Threads                                | Text, Media Threads              | Requires Threads API permissions and account authorization.                                                                                        |
| YouTube                                | Video Upload, Metadata           | Requires Google OAuth and YouTube Data API quota.                                                                                                  |
| TikTok                                 | Video/Image Publishing           | Requires Content Posting API permissions and application review.                                                                                   |
| Pinterest                              | Pin Publishing                   | Requires OAuth and board permissions.                                                                                                              |
| Reddit                                 | Subreddit Posts, Comments        | Requires OAuth and compliance with subreddit rules.                                                                                                |
| Discord / Slack / Telegram             | Channel or Group Messages        | More like message distribution than traditional content platform publishing.                                                                       |
| Mastodon / Bluesky                     | Short-form Text, Images, Video   | High API openness; suitable as standard API publishers.                                                                                            |
| WordPress / Medium / Hashnode / Dev.to | Article Publishing               | Usually available via official API or user API key.                                                                                                |

## Platforms Requiring or Better Suited for Extension DOM

| Platform                   | Recommended Method       | Reason                                                                                                    |
| -------------------------- | ------------------------ | --------------------------------------------------------------------------------------------------------- |
| Zhihu                      | Extension DOM            | No stable open API for articles/answers for ordinary creators. Backend `ZhihuPublisher` is a placeholder. |
| Xiaohongshu                | Extension DOM            | No stable public API for regular notes; better to simulate uploads in the creator center.                 |
| Bilibili Dynamic           | Extension DOM            | Open platform focuses on video/articles; dynamic publishing capabilities require separate confirmation.   |
| Douyin Creator Center      | Extension DOM or Manual  | Ordinary creator publishing interfaces are not suitable for initial API commitment.                       |
| Kuaishou Creator Center    | Extension DOM or Manual  | Automation for ordinary accounts is better suited for browser extensions or manual steps.                 |
| Weibo (Standard Posts)     | API Limited or Extension | Strict write permissions and review; use extension if official write access is unavailable.               |
| Toutiao / Baijiahao / etc. | Extension DOM or Manual  | Mostly media-focused backends; no stable public API for ordinary developers.                              |
| Jianshu / Douban           | Extension DOM or Manual  | Publishing for ordinary accounts is better handled via browser extension simulation or manual steps.      |
