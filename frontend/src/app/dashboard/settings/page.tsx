"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import {
  getWechatAccount,
  saveWechatAccount,
  testWechatConnection,
  type WechatAccount,
  type WechatConnectionTestResult,
} from "@/lib/dashboard/api";
import { SettingsPageHeader } from "./_components/settings-page-header";
import { WechatAccountCard } from "./_components/wechat-account-card";
import { WechatConnectionCheckCard } from "./_components/wechat-connection-check-card";

export default function SettingsPage() {
  const [account, setAccount] = useState<WechatAccount | null>(null);
  const [testResult, setTestResult] =
    useState<WechatConnectionTestResult | null>(null);
  const [appID, setAppID] = useState("");
  const [appSecret, setAppSecret] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function loadAccount() {
      try {
        const response = await getWechatAccount();
        if (cancelled) {
          return;
        }
        setAccount(response);
        setAppID(response.app_id ?? "");
      } catch (error) {
        toast.error("无法加载公众号账号", {
          description:
            error instanceof Error ? error.message : "请稍后刷新页面。",
        });
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void loadAccount();

    return () => {
      cancelled = true;
    };
  }, []);

  const canSubmit = useMemo(() => {
    return Boolean(
      appID.trim() && (appSecret.trim() || account?.has_app_secret),
    );
  }, [account?.has_app_secret, appID, appSecret]);

  const currentIPHint = testResult?.ip_whitelist ?? account?.ip_whitelist;
  const currentAuthHint = testResult?.account_auth ?? account?.account_auth;
  const currentStatus = testResult?.status ?? account?.status ?? "unconfigured";
  const currentLastTestedAt = testResult?.tested_at ?? account?.last_tested_at;
  let currentTestError = account?.last_test_error;
  if (testResult) {
    currentTestError = testResult.connected ? undefined : testResult.message;
  }

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

  return (
    <div className="flex max-w-6xl flex-col gap-4">
      <SettingsPageHeader status={currentStatus} />

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
    </div>
  );
}
