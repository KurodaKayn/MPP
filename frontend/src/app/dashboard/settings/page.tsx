"use client";

import { useEffect, useMemo, useState, type ComponentProps } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  KeyRound,
  Loader2,
  RefreshCw,
  Save,
  ShieldCheck,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  getWechatAccount,
  saveWechatAccount,
  testWechatConnection,
  type RequirementStatus,
  type WechatAccount,
  type WechatConnectionTestResult,
} from "@/lib/dashboard/api";
import { cn } from "@/lib/utils";

const statusLabel: Record<WechatAccount["status"], string> = {
  connected: "已连接",
  failed: "测试失败",
  unconfigured: "未配置",
  untested: "待测试",
};

const statusVariant: Record<
  WechatAccount["status"],
  ComponentProps<typeof Badge>["variant"]
> = {
  connected: "default",
  failed: "destructive",
  unconfigured: "outline",
  untested: "secondary",
};

function formatDate(value?: string) {
  if (!value) {
    return "尚未测试";
  }

  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function hintStyle(status: RequirementStatus["status"]) {
  switch (status) {
    case "passed":
      return {
        className: "border-emerald-200 bg-emerald-50 text-emerald-950",
        icon: CheckCircle2,
      };
    case "failed":
      return {
        className: "border-destructive/25 bg-destructive/10 text-destructive",
        icon: AlertTriangle,
      };
    case "warning":
      return {
        className: "border-amber-200 bg-amber-50 text-amber-950",
        icon: AlertTriangle,
      };
    default:
      return {
        className: "border-border bg-muted/40 text-muted-foreground",
        icon: CircleDashed,
      };
  }
}

function RequirementPanel({ hint }: { hint: RequirementStatus }) {
  const style = hintStyle(hint.status);
  const Icon = style.icon;

  return (
    <div className={cn("rounded-lg border p-4", style.className)}>
      <div className="flex items-start gap-3">
        <Icon className="mt-0.5 size-4 shrink-0" />
        <div className="min-w-0 space-y-1">
          <div className="text-sm font-medium">{hint.title}</div>
          <p className="text-sm leading-6 opacity-90">{hint.message}</p>
        </div>
      </div>
    </div>
  );
}

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
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-3xl font-bold">设置</h2>
          <p className="text-muted-foreground">
            管理微信公众号接口凭证和发布前置条件。
          </p>
        </div>
        <Badge variant={statusVariant[currentStatus]}>
          {statusLabel[currentStatus]}
        </Badge>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(320px,0.85fr)]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <KeyRound className="size-4" />
              微信公众号
            </CardTitle>
            <CardDescription>
              使用公众平台开发者凭证连接官方接口。
            </CardDescription>
            <CardAction>
              <Badge variant="outline">Official API</Badge>
            </CardAction>
          </CardHeader>
          <CardContent>
            <div className="grid gap-5">
              <div className="grid gap-2">
                <Label htmlFor="wechat-app-id">AppID</Label>
                <Input
                  id="wechat-app-id"
                  autoComplete="off"
                  value={appID}
                  onChange={(event) => {
                    setAppID(event.target.value);
                    setTestResult(null);
                  }}
                  placeholder="wx..."
                  disabled={loading || saving || testing}
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="wechat-app-secret">AppSecret</Label>
                <Input
                  id="wechat-app-secret"
                  autoComplete="new-password"
                  type="password"
                  value={appSecret}
                  onChange={(event) => {
                    setAppSecret(event.target.value);
                    setTestResult(null);
                  }}
                  placeholder={
                    account?.has_app_secret
                      ? "已保存，留空则继续沿用"
                      : "请输入 AppSecret"
                  }
                  disabled={loading || saving || testing}
                />
                <p className="text-xs leading-5 text-muted-foreground">
                  AppSecret 不会在页面回显；保存后再次修改时重新填写即可。
                </p>
              </div>

              <div className="flex flex-col gap-2 sm:flex-row">
                <Button
                  type="button"
                  onClick={handleSave}
                  disabled={loading || saving || testing || !canSubmit}
                >
                  {saving ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Save className="size-4" />
                  )}
                  保存配置
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={handleTest}
                  disabled={loading || saving || testing || !canSubmit}
                >
                  {testing ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <RefreshCw className="size-4" />
                  )}
                  测试连接
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="size-4" />
              连接检查
            </CardTitle>
            <CardDescription>
              最近测试：{formatDate(currentLastTestedAt)}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid gap-3">
              {currentIPHint ? <RequirementPanel hint={currentIPHint} /> : null}
              {currentAuthHint ? (
                <RequirementPanel hint={currentAuthHint} />
              ) : null}
              {currentTestError ? (
                <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-4 text-sm leading-6 text-destructive">
                  {currentTestError}
                </div>
              ) : null}
              {testResult?.err_code ? (
                <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
                  微信错误码：{testResult.err_code}
                  {testResult.err_msg ? ` / ${testResult.err_msg}` : ""}
                </div>
              ) : null}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
