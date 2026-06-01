"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";

import {
  cancelBrowserSession,
  completeBrowserSession,
  getBrowserSession,
  getDouyinAccount,
  getWechatAccount,
  getXAccount,
  getZhihuAccount,
  saveWechatAccount,
  saveXAccount,
  startBrowserSession,
  testWechatConnection,
  testXConnection,
  type BrowserSession,
  type DouyinAccount,
  type RequirementStatus,
  type WechatAccount,
  type WechatConnectionTestResult,
  type XAccount,
  type XConnectionTestResult,
  type ZhihuAccount,
} from "@/lib/dashboard/api";

type AccountStatus = "unconfigured" | "untested" | "connected" | "failed";
type BrowserAccountPlatform = "douyin" | "zhihu";

type ConnectionCheck = {
  authHint?: RequirementStatus;
  errCode?: number;
  errMsg?: string;
  ipHint?: RequirementStatus;
  lastTestedAt?: string;
  testError?: string;
};

const browserPlatformLabels: Record<BrowserAccountPlatform, string> = {
  douyin: "抖音",
  zhihu: "知乎",
};

const finalBrowserSessionStatuses = new Set<BrowserSession["status"]>([
  "connected",
  "expired",
  "failed",
]);

function getErrorDescription(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function getAggregateStatus(statuses: AccountStatus[]): AccountStatus {
  if (statuses.some((status) => status === "connected")) {
    return "connected";
  }

  if (statuses.some((status) => status === "failed")) {
    return "failed";
  }

  if (statuses.some((status) => status === "untested")) {
    return "untested";
  }

  return "unconfigured";
}

function getBrowserAccountPlatform(
  session: BrowserSession,
): BrowserAccountPlatform {
  return session.platform === "zhihu" ? "zhihu" : "douyin";
}

function buildBrowserConnectionCheck(
  account: DouyinAccount | ZhihuAccount | null,
  platformLabel: string,
): ConnectionCheck {
  const connected = account?.status === "connected";

  return {
    authHint: {
      message: connected
        ? "已保存加密 Cookie，发布时会在服务边界解密后交给发布器。"
        : "点击连接后在官方页面完成扫码或登录。",
      status: connected ? "passed" : "unknown",
      title: "Cookie 发布凭据",
    },
    ipHint: {
      message: `${platformLabel}使用隔离 Chromium 登录，不需要手工复制 Cookie 或输入密码到 MPP 表单。`,
      status: connected ? "passed" : "unknown",
      title: "远程浏览器连接",
    },
    lastTestedAt: account?.updated_at,
    testError: account?.last_test_error,
  };
}

export function useAuthPageController() {
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
          description: getErrorDescription(error, "请稍后刷新页面。"),
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

    if (finalBrowserSessionStatuses.has(browserSession.status)) {
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
            setBrowserStreamURL(
              (currentStreamURL) => currentStreamURL ?? nextSession.stream_url,
            );
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
            setBrowserError(getErrorDescription(error, "无法刷新会话状态。"));
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
  const currentDouyinStatus = douyinAccount?.status ?? "unconfigured";
  const currentZhihuStatus = zhihuAccount?.status ?? "unconfigured";
  const accountStatuses = [
    currentStatus,
    currentXStatus,
    currentDouyinStatus,
    currentZhihuStatus,
  ];
  const connectedCount = accountStatuses.filter(
    (status) => status === "connected",
  ).length;
  const aggregateStatus = getAggregateStatus(accountStatuses);

  let currentTestError = account?.last_test_error;
  if (testResult) {
    currentTestError = testResult.connected ? undefined : testResult.message;
  }

  let currentXTestError = xAccount?.last_test_error;
  if (xTestResult) {
    currentXTestError = xTestResult.connected ? undefined : xTestResult.message;
  }

  const handleAppIDChange = (value: string) => {
    setAppID(value);
    setTestResult(null);
  };

  const handleAppSecretChange = (value: string) => {
    setAppSecret(value);
    setTestResult(null);
  };

  const handleXAPIKeyChange = (value: string) => {
    setXAPIKey(value);
    setXTestResult(null);
  };

  const handleXAPISecretChange = (value: string) => {
    setXAPISecret(value);
    setXTestResult(null);
  };

  const handleXAccessTokenChange = (value: string) => {
    setXAccessToken(value);
    setXTestResult(null);
  };

  const handleXAccessTokenSecretChange = (value: string) => {
    setXAccessTokenSecret(value);
    setXTestResult(null);
  };

  const handleXUsernameChange = (value: string) => {
    setXUsername(value);
    setXTestResult(null);
  };

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
        description: getErrorDescription(error, "请检查输入。"),
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
        description: getErrorDescription(error, "请检查输入。"),
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
        description: getErrorDescription(error, "请检查输入。"),
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
        description: getErrorDescription(error, "请检查输入。"),
      });
    } finally {
      setXTesting(false);
    }
  };

  const handleConnectBrowserAccount = async (
    platform: BrowserAccountPlatform,
  ) => {
    const setConnecting =
      platform === "zhihu" ? setZhihuConnecting : setDouyinConnecting;
    const platformLabel = browserPlatformLabels[platform];

    setConnecting(true);
    setBrowserError(undefined);
    try {
      const session = await startBrowserSession(platform);
      setBrowserSession({
        expires_at: session.expires_at,
        platform,
        session_id: session.session_id,
        status: session.status,
        stream_token_expires_at: session.stream_token_expires_at,
        stream_url: session.stream_url,
      });
      setBrowserStreamURL(session.stream_url);
      toast.success("远程浏览器已启动", {
        description: `请在弹窗中完成${platformLabel}登录。`,
      });
    } catch (error) {
      toast.error(`无法启动${platformLabel}连接`, {
        description: getErrorDescription(error, "请稍后再试。"),
      });
    } finally {
      setConnecting(false);
    }
  };

  const handleCompleteBrowserAccount = async (
    platform: BrowserAccountPlatform,
  ) => {
    if (!browserSession) {
      return;
    }

    const setCompleting =
      platform === "zhihu" ? setZhihuCompleting : setDouyinCompleting;
    const platformLabel = browserPlatformLabels[platform];

    setCompleting(true);
    setBrowserError(undefined);
    try {
      const result = await completeBrowserSession(browserSession.session_id);
      if (platform === "zhihu") {
        const platformAccount = await getZhihuAccount();
        setZhihuAccount(platformAccount);
      } else {
        const platformAccount = await getDouyinAccount();
        setDouyinAccount(platformAccount);
      }
      setBrowserSession(null);
      setBrowserStreamURL(undefined);
      toast.success(`${platformLabel}账号已连接`, {
        description: result.account.username || "Cookie 已安全保存。",
      });
    } catch (error) {
      setBrowserError(getErrorDescription(error, "还没有检测到登录状态。"));
    } finally {
      setCompleting(false);
    }
  };

  const handleCancelBrowserSession = async () => {
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
        description: getErrorDescription(error, "浏览器会话可能已经结束。"),
      });
    }
  };

  const activeBrowserAccountPlatform = browserSession
    ? getBrowserAccountPlatform(browserSession)
    : null;

  return {
    browserSession:
      browserSession && activeBrowserAccountPlatform
        ? {
            completing:
              activeBrowserAccountPlatform === "zhihu"
                ? zhihuCompleting
                : douyinCompleting,
            error: browserError,
            expiresAt: browserSession.expires_at,
            platformLabel: browserPlatformLabels[activeBrowserAccountPlatform],
            status: browserSession.status,
            streamURL: browserStreamURL,
            onCancel: handleCancelBrowserSession,
            onComplete: () => {
              void handleCompleteBrowserAccount(activeBrowserAccountPlatform);
            },
          }
        : null,
    douyin: {
      account: douyinAccount,
      connectionCheck: buildBrowserConnectionCheck(douyinAccount, "抖音"),
      connecting: douyinConnecting,
      loading,
      onConnect: () => {
        void handleConnectBrowserAccount("douyin");
      },
    },
    header: {
      connectedCount,
      status: aggregateStatus,
      totalCount: 4,
    },
    wechat: {
      account,
      appID,
      appSecret,
      canSubmit,
      connectionCheck: {
        authHint: currentAuthHint,
        errCode: testResult?.err_code,
        errMsg: testResult?.err_msg,
        ipHint: currentIPHint,
        lastTestedAt: currentLastTestedAt,
        testError: currentTestError,
      },
      loading,
      saving,
      testing,
      onAppIDChange: handleAppIDChange,
      onAppSecretChange: handleAppSecretChange,
      onSave: () => {
        void handleSave();
      },
      onTest: () => {
        void handleTest();
      },
    },
    x: {
      accessToken: xAccessToken,
      accessTokenSecret: xAccessTokenSecret,
      account: xAccount,
      apiKey: xAPIKey,
      apiSecret: xAPISecret,
      canSubmit: canSubmitX,
      connectionCheck: {
        authHint: currentXPublishHint,
        ipHint: currentXAccountAuthHint,
        lastTestedAt: currentXLastTestedAt,
        testError: currentXTestError,
      },
      loading,
      saving: xSaving,
      testing: xTesting,
      username: xUsername,
      onAPIKeyChange: handleXAPIKeyChange,
      onAPISecretChange: handleXAPISecretChange,
      onAccessTokenChange: handleXAccessTokenChange,
      onAccessTokenSecretChange: handleXAccessTokenSecretChange,
      onSave: () => {
        void handleSaveX();
      },
      onTest: () => {
        void handleTestX();
      },
      onUsernameChange: handleXUsernameChange,
    },
    zhihu: {
      account: zhihuAccount,
      connectionCheck: buildBrowserConnectionCheck(zhihuAccount, "知乎"),
      connecting: zhihuConnecting,
      loading,
      onConnect: () => {
        void handleConnectBrowserAccount("zhihu");
      },
    },
  };
}
