"use client";

import { Suspense, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";

import {
  cancelBrowserSession,
  completeBrowserSession,
  getBrowserSession,
  getDouyinAccount,
  getZhihuAccount,
  getXAccount,
  getWechatAccount,
  saveWechatAccount,
  saveXAccount,
  startBrowserSession,
  testWechatConnection,
  testXConnection,
  type BrowserSession,
  type DouyinAccount,
  type ZhihuAccount,
  type WechatAccount,
  type WechatConnectionTestResult,
  type XAccount,
  type XConnectionTestResult,
} from "@/lib/dashboard/api";
import { AuthPageHeader } from "./_components/auth-page-header";
import { WechatAccountCard } from "./_components/wechat-account-card";
import { WechatConnectionCheckCard } from "./_components/wechat-connection-check-card";
import { DouyinAccountCard } from "./_components/douyin-account-card";
import { ZhihuAccountCard } from "./_components/zhihu-account-card";
import { RemoteBrowserSessionModal } from "./_components/remote-browser-session-modal";
import { XAccountCard } from "./_components/x-account-card";

function AuthPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [account, setAccount] = useState<WechatAccount | null>(null);
  const [testResult, setTestResult] =
    useState<WechatConnectionTestResult | null>(null);
  const [xAccount, setXAccount] = useState<XAccount | null>(null);
  const [douyinAccount, setDouyinAccount] = useState<DouyinAccount | null>(
    null,
  );
  const [zhihuAccount, setZhihuAccount] = useState<ZhihuAccount | null>(null);
  const [browserSession, setBrowserSession] = useState<BrowserSession | null>(
    null,
  );
  const [browserStreamURL, setBrowserStreamURL] = useState<string>();
  const [browserError, setBrowserError] = useState<string>();
  const [xTestResult, setXTestResult] = useState<XConnectionTestResult | null>(
    null,
  );
  const [appID, setAppID] = useState("");
  const [appSecret, setAppSecret] = useState("");
  const [xAPIKey, setXAPIKey] = useState("");
  const [xAPISecret, setXAPISecret] = useState("");
  const [xAccessToken, setXAccessToken] = useState("");
  const [xAccessTokenSecret, setXAccessTokenSecret] = useState("");
  const [xUsername, setXUsername] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [xSaving, setXSaving] = useState(false);
  const [xTesting, setXTesting] = useState(false);
  const [douyinConnecting, setDouyinConnecting] = useState(false);
  const [douyinCompleting, setDouyinCompleting] = useState(false);
  const [zhihuConnecting, setZhihuConnecting] = useState(false);
  const [zhihuCompleting, setZhihuCompleting] = useState(false);
  const xOAuthStatus = searchParams.get("x_oauth");

  useEffect(() => {
    let cancelled = false;

    async function loadAccounts() {
      try {
        const [wechatResponse, xResponse, douyinResponse, zhihuResponse] =
          await Promise.all([
            getWechatAccount(),
            getXAccount(),
            getDouyinAccount(),
            getZhihuAccount(),
          ]);
        if (cancelled) {
          return;
        }
        setAccount(wechatResponse);
        setAppID(wechatResponse.app_id ?? "");
        setXAccount(xResponse);
        setDouyinAccount(douyinResponse);
        setZhihuAccount(zhihuResponse);
        setXAPIKey(xResponse.api_key ?? "");
        setXUsername(xResponse.username ?? "");
      } catch (error) {
        toast.error("无法加载平台账号", {
          description:
            error instanceof Error ? error.message : "请稍后刷新页面。",
        });
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void loadAccounts();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!browserSession) {
      return;
    }

    const finalStatuses = new Set(["connected", "expired", "failed"]);
    if (finalStatuses.has(browserSession.status)) {
      return;
    }

    let cancelled = false;
    const interval = window.setInterval(() => {
      void getBrowserSession(browserSession.session_id)
        .then((nextSession) => {
          if (cancelled) {
            return;
          }
          setBrowserSession(nextSession);
          if (nextSession.stream_url) {
            setBrowserStreamURL(nextSession.stream_url);
          }
          if (nextSession.status === "expired") {
            setBrowserError("远程浏览器会话已过期，请重新连接。");
          }
          if (nextSession.status === "failed") {
            setBrowserError(nextSession.message || "远程浏览器启动失败。");
          }
        })
        .catch((error) => {
          if (!cancelled) {
            setBrowserError(
              error instanceof Error ? error.message : "无法刷新会话状态。",
            );
          }
        });
    }, 2500);

    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [browserSession]);

  useEffect(() => {
    if (xOAuthStatus === "connected") {
      toast.success("X 授权成功", {
        description: "账号已连接，正在更新设置。",
      });
      router.replace("/dashboard/settings");
      return;
    }

    if (xOAuthStatus === "failed") {
      toast.error("X 授权失败", {
        description: "请重新点击授权按钮。",
      });
      router.replace("/dashboard/settings");
    }
  }, [router, xOAuthStatus]);

  const canSubmit = useMemo(() => {
    return Boolean(
      appID.trim() && (appSecret.trim() || account?.has_app_secret),
    );
  }, [account?.has_app_secret, appID, appSecret]);
  const canSubmitX = useMemo(() => {
    return Boolean(
      (xAPIKey.trim() || xAccount?.api_key) &&
      (xAPISecret.trim() || xAccount?.has_api_secret) &&
      (xAccessToken.trim() || xAccount?.has_access_token) &&
      (xAccessTokenSecret.trim() || xAccount?.has_access_token_secret),
    );
  }, [
    xAPIKey,
    xAPISecret,
    xAccessToken,
    xAccessTokenSecret,
    xAccount?.api_key,
    xAccount?.has_access_token,
    xAccount?.has_access_token_secret,
    xAccount?.has_api_secret,
  ]);

  const currentIPHint = testResult?.ip_whitelist ?? account?.ip_whitelist;
  const currentAuthHint = testResult?.account_auth ?? account?.account_auth;
  const currentStatus = testResult?.status ?? account?.status ?? "unconfigured";
  const currentLastTestedAt = testResult?.tested_at ?? account?.last_tested_at;
  const currentXAccountAuthHint =
    xTestResult?.account_auth ?? xAccount?.account_auth;
  const currentXPublishHint =
    xTestResult?.publish_access ?? xAccount?.publish_access;
  const currentXStatus =
    xTestResult?.status ?? xAccount?.status ?? "unconfigured";
  const currentXLastTestedAt =
    xTestResult?.tested_at ?? xAccount?.last_tested_at;
  let currentTestError = account?.last_test_error;
  if (testResult) {
    currentTestError = testResult.connected ? undefined : testResult.message;
  }
  let currentXTestError = xAccount?.last_test_error;
  if (xTestResult) {
    currentXTestError = xTestResult.connected ? undefined : xTestResult.message;
  }
  const currentDouyinStatus = douyinAccount?.status ?? "unconfigured";
  const currentZhihuStatus = zhihuAccount?.status ?? "unconfigured";
  const connectedCount = [
    currentStatus,
    currentXStatus,
    currentDouyinStatus,
    currentZhihuStatus,
  ].filter((status) => status === "connected").length;
  const aggregateStatus =
    connectedCount > 0
      ? "connected"
      : currentStatus === "failed" ||
          currentXStatus === "failed" ||
          currentDouyinStatus === "failed" ||
          currentZhihuStatus === "failed"
        ? "failed"
        : currentStatus === "untested" ||
            currentXStatus === "untested" ||
            currentDouyinStatus === "untested" ||
            currentZhihuStatus === "untested"
          ? "untested"
          : "unconfigured";

  const handleSave = async () => {
    setSaving(true);
    try {
      const response = await saveWechatAccount({
        app_id: appID.trim(),
        app_secret: appSecret.trim(),
      });
      setAccount(response);
      setTestResult(null);
      setAppSecret("");
      toast.success("公众号账号已保存");
    } catch (error) {
      toast.error("保存失败", {
        description: error instanceof Error ? error.message : "请检查输入。",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    setTesting(true);
    try {
      const result = await testWechatConnection({
        app_id: appID.trim(),
        app_secret: appSecret.trim(),
      });
      setTestResult(result);

      if (result.connected) {
        toast.success("连接成功", {
          description: "微信接口已接受当前 AppID/AppSecret。",
        });
      } else {
        toast.error("连接失败", {
          description: result.message,
        });
      }
    } catch (error) {
      toast.error("测试失败", {
        description: error instanceof Error ? error.message : "请检查输入。",
      });
    } finally {
      setTesting(false);
    }
  };
  const handleSaveX = async () => {
    setXSaving(true);
    try {
      const response = await saveXAccount({
        access_token: xAccessToken.trim(),
        access_token_secret: xAccessTokenSecret.trim(),
        api_key: xAPIKey.trim(),
        api_secret: xAPISecret.trim(),
        username: xUsername.trim(),
      });
      setXAccount(response);
      setXTestResult(null);
      setXAPIKey(response.api_key ?? "");
      setXUsername(response.username ?? xUsername.trim());
      setXAPISecret("");
      setXAccessToken("");
      setXAccessTokenSecret("");
      toast.success("X 账号已保存");
    } catch (error) {
      toast.error("保存失败", {
        description: error instanceof Error ? error.message : "请检查输入。",
      });
    } finally {
      setXSaving(false);
    }
  };

  const handleTestX = async () => {
    setXTesting(true);
    try {
      const result = await testXConnection({
        access_token: xAccessToken.trim(),
        access_token_secret: xAccessTokenSecret.trim(),
        api_key: xAPIKey.trim(),
        api_secret: xAPISecret.trim(),
      });
      setXTestResult(result);
      if (result.username) {
        setXUsername(result.username);
      }

      if (result.connected) {
        toast.success("X 连接成功", {
          description: result.username
            ? `已连接 @${result.username}`
            : "X API 已接受当前凭证。",
        });
      } else {
        toast.error("X 连接失败", {
          description: result.message,
        });
      }
    } catch (error) {
      toast.error("测试失败", {
        description: error instanceof Error ? error.message : "请检查输入。",
      });
    } finally {
      setXTesting(false);
    }
  };

  const handleConnectDouyin = async () => {
    setDouyinConnecting(true);
    setBrowserError(undefined);
    try {
      const session = await startBrowserSession("douyin");
      setBrowserSession({
        expires_at: session.expires_at,
        platform: "douyin",
        session_id: session.session_id,
        status: session.status,
        stream_token_expires_at: session.stream_token_expires_at,
        stream_url: session.stream_url,
      });
      setBrowserStreamURL(session.stream_url);
      toast.success("远程浏览器已启动", {
        description: "请在弹窗中完成抖音登录。",
      });
    } catch (error) {
      toast.error("无法启动抖音连接", {
        description: error instanceof Error ? error.message : "请稍后再试。",
      });
    } finally {
      setDouyinConnecting(false);
    }
  };

  const handleCompleteDouyin = async () => {
    if (!browserSession) {
      return;
    }

    setDouyinCompleting(true);
    setBrowserError(undefined);
    try {
      const result = await completeBrowserSession(browserSession.session_id);
      const account = await getDouyinAccount();
      setDouyinAccount(account);
      setBrowserSession(null);
      setBrowserStreamURL(undefined);
      toast.success("抖音账号已连接", {
        description: result.account.username || "Cookie 已安全保存。",
      });
    } catch (error) {
      setBrowserError(
        error instanceof Error ? error.message : "还没有检测到登录状态。",
      );
    } finally {
      setDouyinCompleting(false);
    }
  };

  const handleCancelDouyin = async () => {
    const sessionID = browserSession?.session_id;
    setBrowserSession(null);
    setBrowserStreamURL(undefined);
    setBrowserError(undefined);

    if (!sessionID) {
      return;
    }

    try {
      await cancelBrowserSession(sessionID);
    } catch (error) {
      toast.error("取消远程会话失败", {
        description:
          error instanceof Error ? error.message : "浏览器会话可能已经结束。",
      });
    }
  };

  const handleConnectZhihu = async () => {
    setZhihuConnecting(true);
    setBrowserError(undefined);
    try {
      const session = await startBrowserSession("zhihu");
      setBrowserSession({
        expires_at: session.expires_at,
        platform: "zhihu",
        session_id: session.session_id,
        status: session.status,
        stream_token_expires_at: session.stream_token_expires_at,
        stream_url: session.stream_url,
      });
      setBrowserStreamURL(session.stream_url);
      toast.success("远程浏览器已启动", {
        description: "请在弹窗中完成知乎登录。",
      });
    } catch (error) {
      toast.error("无法启动知乎连接", {
        description: error instanceof Error ? error.message : "请稍后再试。",
      });
    } finally {
      setZhihuConnecting(false);
    }
  };

  const handleCompleteZhihu = async () => {
    if (!browserSession) {
      return;
    }

    setZhihuCompleting(true);
    setBrowserError(undefined);
    try {
      const result = await completeBrowserSession(browserSession.session_id);
      const account = await getZhihuAccount();
      setZhihuAccount(account);
      setBrowserSession(null);
      setBrowserStreamURL(undefined);
      toast.success("知乎账号已连接", {
        description: result.account.username || "Cookie 已安全保存。",
      });
    } catch (error) {
      setBrowserError(
        error instanceof Error ? error.message : "还没有检测到登录状态。",
      );
    } finally {
      setZhihuCompleting(false);
    }
  };

  const handleCancelZhihu = async () => {
    const sessionID = browserSession?.session_id;
    setBrowserSession(null);
    setBrowserStreamURL(undefined);
    setBrowserError(undefined);

    if (!sessionID) {
      return;
    }

    try {
      await cancelBrowserSession(sessionID);
    } catch (error) {
      toast.error("取消远程会话失败", {
        description:
          error instanceof Error ? error.message : "浏览器会话可能已经结束。",
      });
    }
  };

  return (
    <div className="mx-auto w-full flex max-w-6xl flex-col gap-4">
      <AuthPageHeader
        connectedCount={connectedCount}
        status={aggregateStatus}
        totalCount={4}
      />

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <WechatAccountCard
          account={account}
          appID={appID}
          appSecret={appSecret}
          loading={loading}
          saving={saving}
          testing={testing}
          canSubmit={canSubmit}
          onAppIDChange={(value) => {
            setAppID(value);
            setTestResult(null);
          }}
          onAppSecretChange={(value) => {
            setAppSecret(value);
            setTestResult(null);
          }}
          onSave={handleSave}
          onTest={handleTest}
        />

        <WechatConnectionCheckCard
          lastTestedAt={currentLastTestedAt}
          ipHint={currentIPHint}
          authHint={currentAuthHint}
          testError={currentTestError}
          errCode={testResult?.err_code}
          errMsg={testResult?.err_msg}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <DouyinAccountCard
          account={douyinAccount}
          connecting={douyinConnecting}
          loading={loading}
          onConnect={handleConnectDouyin}
        />

        <WechatConnectionCheckCard
          lastTestedAt={douyinAccount?.updated_at}
          ipHint={{
            message:
              "抖音使用隔离 Chromium 登录，不需要手工复制 Cookie 或输入密码到 MPP 表单。",
            status:
              douyinAccount?.status === "connected" ? "passed" : "unknown",
            title: "远程浏览器连接",
          }}
          authHint={{
            message:
              douyinAccount?.status === "connected"
                ? "已保存加密 Cookie，发布时会在服务边界解密后交给发布器。"
                : "点击连接后在官方页面完成扫码或登录。",
            status:
              douyinAccount?.status === "connected" ? "passed" : "unknown",
            title: "Cookie 发布凭据",
          }}
          testError={douyinAccount?.last_test_error}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <ZhihuAccountCard
          account={zhihuAccount}
          connecting={zhihuConnecting}
          loading={loading}
          onConnect={handleConnectZhihu}
        />

        <WechatConnectionCheckCard
          lastTestedAt={zhihuAccount?.updated_at}
          ipHint={{
            message:
              "知乎使用隔离 Chromium 登录，不需要手工复制 Cookie 或输入密码到 MPP 表单。",
            status: zhihuAccount?.status === "connected" ? "passed" : "unknown",
            title: "远程浏览器连接",
          }}
          authHint={{
            message:
              zhihuAccount?.status === "connected"
                ? "已保存加密 Cookie，发布时会在服务边界解密后交给发布器。"
                : "点击连接后在官方页面完成扫码或登录。",
            status: zhihuAccount?.status === "connected" ? "passed" : "unknown",
            title: "Cookie 发布凭据",
          }}
          testError={zhihuAccount?.last_test_error}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <XAccountCard
          account={xAccount}
          accessToken={xAccessToken}
          accessTokenSecret={xAccessTokenSecret}
          apiKey={xAPIKey}
          apiSecret={xAPISecret}
          username={xUsername}
          loading={loading}
          saving={xSaving}
          testing={xTesting}
          canSubmit={canSubmitX}
          onAPIKeyChange={(value) => {
            setXAPIKey(value);
            setXTestResult(null);
          }}
          onAPISecretChange={(value) => {
            setXAPISecret(value);
            setXTestResult(null);
          }}
          onAccessTokenChange={(value) => {
            setXAccessToken(value);
            setXTestResult(null);
          }}
          onAccessTokenSecretChange={(value) => {
            setXAccessTokenSecret(value);
            setXTestResult(null);
          }}
          onUsernameChange={(value) => {
            setXUsername(value);
            setXTestResult(null);
          }}
          onSave={handleSaveX}
          onTest={handleTestX}
        />

        <WechatConnectionCheckCard
          lastTestedAt={currentXLastTestedAt}
          ipHint={currentXAccountAuthHint}
          authHint={currentXPublishHint}
          testError={currentXTestError}
        />
      </div>

      {browserSession ? (
        <RemoteBrowserSessionModal
          completing={
            browserSession.platform === "zhihu"
              ? zhihuCompleting
              : douyinCompleting
          }
          error={browserError}
          expiresAt={browserSession.expires_at}
          platformLabel={browserSession.platform === "zhihu" ? "知乎" : "抖音"}
          status={browserSession.status}
          streamURL={browserStreamURL}
          onCancel={
            browserSession.platform === "zhihu"
              ? handleCancelZhihu
              : handleCancelDouyin
          }
          onComplete={
            browserSession.platform === "zhihu"
              ? handleCompleteZhihu
              : handleCompleteDouyin
          }
        />
      ) : null}
    </div>
  );
}

export default function AuthPage() {
  return (
    <Suspense>
      <AuthPageContent />
    </Suspense>
  );
}
