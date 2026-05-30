"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import {
  getXAccount,
  getWechatAccount,
  saveWechatAccount,
  saveXAccount,
  testWechatConnection,
  testXConnection,
  type WechatAccount,
  type WechatConnectionTestResult,
  type XAccount,
  type XConnectionTestResult,
} from "@/lib/dashboard/api";
import { SettingsPageHeader } from "./_components/settings-page-header";
import { WechatAccountCard } from "./_components/wechat-account-card";
import { WechatConnectionCheckCard } from "./_components/wechat-connection-check-card";
import { XAccountCard } from "./_components/x-account-card";

export default function SettingsPage() {
  const [account, setAccount] = useState<WechatAccount | null>(null);
  const [testResult, setTestResult] =
    useState<WechatConnectionTestResult | null>(null);
  const [xAccount, setXAccount] = useState<XAccount | null>(null);
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

  useEffect(() => {
    let cancelled = false;

    async function loadAccounts() {
      try {
        const [wechatResponse, xResponse] = await Promise.all([
          getWechatAccount(),
          getXAccount(),
        ]);
        if (cancelled) {
          return;
        }
        setAccount(wechatResponse);
        setAppID(wechatResponse.app_id ?? "");
        setXAccount(xResponse);
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
  const connectedCount = [currentStatus, currentXStatus].filter(
    (status) => status === "connected",
  ).length;
  const aggregateStatus =
    connectedCount > 0
      ? "connected"
      : currentStatus === "failed" || currentXStatus === "failed"
        ? "failed"
        : currentStatus === "untested" || currentXStatus === "untested"
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

  return (
    <div className="flex max-w-6xl flex-col gap-4">
      <SettingsPageHeader
        connectedCount={connectedCount}
        status={aggregateStatus}
        totalCount={2}
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
    </div>
  );
}
