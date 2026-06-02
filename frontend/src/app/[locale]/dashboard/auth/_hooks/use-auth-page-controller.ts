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
import { useTranslation, useAppLocale } from "@/lib/i18n/client";

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

export function useAuthPageController() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const { t: tCommon } = useTranslation(locale, "common");

  const browserPlatformLabels: Record<BrowserAccountPlatform, string> = {
    douyin: tCommon("platforms.douyin"),
    zhihu: tCommon("platforms.zhihu"),
  };

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

  function buildBrowserConnectionCheck(
    account: DouyinAccount | ZhihuAccount | null,
    platform: BrowserAccountPlatform,
  ): ConnectionCheck {
    const connected = account?.status === "connected";
    const platformLabel = browserPlatformLabels[platform];

    return {
      authHint: {
        message: connected
          ? t("auth.hints.cookieConnected")
          : t("auth.hints.cookieNotConnected"),
        status: connected ? "passed" : "unknown",
        title: t("auth.hints.cookieAuth"),
      },
      ipHint: {
        message: t("auth.hints.remoteBrowserDesc", { platform: platformLabel }),
        status: connected ? "passed" : "unknown",
        title: t("auth.hints.remoteBrowser"),
      },
      lastTestedAt: account?.updated_at,
      testError: account?.last_test_error,
    };
  }

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
        toast.error(t("auth.toast.loadFailed"), {
          description: getErrorDescription(
            error,
            t("auth.toast.loadFailedDesc"),
          ),
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
  }, [t]);

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
            setBrowserError(t("auth.toast.sessionExpired"));
          }
          if (nextSession.status === "failed") {
            setBrowserError(
              nextSession.message || t("auth.toast.sessionStartFailed"),
            );
          }
        })
        .catch((error) => {
          if (!cancelled) {
            setBrowserError(
              getErrorDescription(error, t("auth.toast.sessionRefreshFailed")),
            );
          }
        });
    }, 2500);

    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [browserSession, t]);

  useEffect(() => {
    if (xOAuthStatus === "connected") {
      toast.success(t("auth.toast.xSuccess"), {
        description: t("auth.toast.xSuccessDesc"),
      });
      router.replace("/dashboard/settings");
      return;
    }

    if (xOAuthStatus === "failed") {
      toast.error(t("auth.toast.xFailed"), {
        description: t("auth.toast.xFailedDesc"),
      });
      router.replace("/dashboard/settings");
    }
  }, [router, xOAuthStatus, t]);

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
      toast.success(t("auth.toast.saveSuccess"));
    } catch (error) {
      toast.error(t("auth.toast.saveFailed"), {
        description: getErrorDescription(error, t("auth.toast.saveFailedDesc")),
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
        toast.success(t("auth.toast.connectSuccess"), {
          description: t("auth.toast.wechatConnectSuccessDesc"),
        });
      } else {
        toast.error(t("auth.toast.connectFailed"), {
          description: result.message,
        });
      }
    } catch (error) {
      toast.error(t("auth.toast.testFailed"), {
        description: getErrorDescription(error, t("auth.toast.saveFailedDesc")),
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
      toast.success(t("auth.toast.saveSuccess"));
    } catch (error) {
      toast.error(t("auth.toast.saveFailed"), {
        description: getErrorDescription(error, t("auth.toast.saveFailedDesc")),
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
        toast.success(t("auth.toast.xConnectSuccess"), {
          description: result.username
            ? t("auth.toast.xConnectedAt", { username: result.username })
            : t("auth.toast.xConnectSuccessDesc"),
        });
      } else {
        toast.error(t("auth.toast.connectFailed"), {
          description: result.message,
        });
      }
    } catch (error) {
      toast.error(t("auth.toast.testFailed"), {
        description: getErrorDescription(error, t("auth.toast.saveFailedDesc")),
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
      toast.success(t("auth.toast.browserStarted"), {
        description: t("auth.toast.browserStartedDesc", {
          platform: platformLabel,
        }),
      });
    } catch (error) {
      toast.error(
        t("auth.toast.browserStartFailed", { platform: platformLabel }),
        {
          description: getErrorDescription(
            error,
            t("auth.toast.browserStartFailedDesc"),
          ),
        },
      );
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
      toast.success(
        t("auth.toast.browserConnected", { platform: platformLabel }),
        {
          description:
            result.account.username || t("auth.toast.browserConnectedDesc"),
        },
      );
    } catch (error) {
      setBrowserError(
        getErrorDescription(error, t("auth.toast.browserLoginNotDetected")),
      );
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
      toast.error(t("auth.toast.browserCancelFailed"), {
        description: getErrorDescription(
          error,
          t("auth.toast.browserCancelFailedDesc"),
        ),
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
      connectionCheck: buildBrowserConnectionCheck(douyinAccount, "douyin"),
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
      connectionCheck: buildBrowserConnectionCheck(zhihuAccount, "zhihu"),
      connecting: zhihuConnecting,
      loading,
      onConnect: () => {
        void handleConnectBrowserAccount("zhihu");
      },
    },
  };
}
