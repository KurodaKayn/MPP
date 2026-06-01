"use client";

import { AuthPageHeader } from "./auth-page-header";
import { DouyinAccountCard } from "./douyin-account-card";
import { RemoteBrowserSessionModal } from "./remote-browser-session-modal";
import { WechatAccountCard } from "./wechat-account-card";
import { WechatConnectionCheckCard } from "./wechat-connection-check-card";
import { XAccountCard } from "./x-account-card";
import { ZhihuAccountCard } from "./zhihu-account-card";
import { useAuthPageController } from "../_hooks/use-auth-page-controller";

export function AuthPageContent() {
  const authPage = useAuthPageController();

  return (
    <div className="mx-auto w-full flex max-w-6xl flex-col gap-4">
      <AuthPageHeader {...authPage.header} />

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <WechatAccountCard
          account={authPage.wechat.account}
          appID={authPage.wechat.appID}
          appSecret={authPage.wechat.appSecret}
          loading={authPage.wechat.loading}
          saving={authPage.wechat.saving}
          testing={authPage.wechat.testing}
          canSubmit={authPage.wechat.canSubmit}
          onAppIDChange={authPage.wechat.onAppIDChange}
          onAppSecretChange={authPage.wechat.onAppSecretChange}
          onSave={authPage.wechat.onSave}
          onTest={authPage.wechat.onTest}
        />

        <WechatConnectionCheckCard {...authPage.wechat.connectionCheck} />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <DouyinAccountCard
          account={authPage.douyin.account}
          connecting={authPage.douyin.connecting}
          loading={authPage.douyin.loading}
          onConnect={authPage.douyin.onConnect}
        />

        <WechatConnectionCheckCard {...authPage.douyin.connectionCheck} />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <ZhihuAccountCard
          account={authPage.zhihu.account}
          connecting={authPage.zhihu.connecting}
          loading={authPage.zhihu.loading}
          onConnect={authPage.zhihu.onConnect}
        />

        <WechatConnectionCheckCard {...authPage.zhihu.connectionCheck} />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <XAccountCard
          account={authPage.x.account}
          accessToken={authPage.x.accessToken}
          accessTokenSecret={authPage.x.accessTokenSecret}
          apiKey={authPage.x.apiKey}
          apiSecret={authPage.x.apiSecret}
          username={authPage.x.username}
          loading={authPage.x.loading}
          saving={authPage.x.saving}
          testing={authPage.x.testing}
          canSubmit={authPage.x.canSubmit}
          onAPIKeyChange={authPage.x.onAPIKeyChange}
          onAPISecretChange={authPage.x.onAPISecretChange}
          onAccessTokenChange={authPage.x.onAccessTokenChange}
          onAccessTokenSecretChange={authPage.x.onAccessTokenSecretChange}
          onUsernameChange={authPage.x.onUsernameChange}
          onSave={authPage.x.onSave}
          onTest={authPage.x.onTest}
        />

        <WechatConnectionCheckCard {...authPage.x.connectionCheck} />
      </div>

      {authPage.browserSession ? (
        <RemoteBrowserSessionModal {...authPage.browserSession} />
      ) : null}
    </div>
  );
}
