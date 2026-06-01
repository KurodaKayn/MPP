"use client";

import { KeyRound, Loader2, RefreshCw, Save } from "lucide-react";

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
import { type WechatAccount } from "@/lib/dashboard/api";
import { useTranslation, useAppLocale } from "@/lib/i18n/client";

import { WechatCredentialGuideDialog } from "./wechat-credential-guide-dialog";

type WechatAccountCardProps = {
  account: WechatAccount | null;
  appID: string;
  appSecret: string;
  loading: boolean;
  saving: boolean;
  testing: boolean;
  canSubmit: boolean;
  onAppIDChange: (value: string) => void;
  onAppSecretChange: (value: string) => void;
  onSave: () => void;
  onTest: () => void;
};

export function WechatAccountCard({
  account,
  appID,
  appSecret,
  loading,
  saving,
  testing,
  canSubmit,
  onAppIDChange,
  onAppSecretChange,
  onSave,
  onTest,
}: WechatAccountCardProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRound className="size-4" />
          {t("auth.wechat.title")}
        </CardTitle>
        <CardDescription>{t("auth.wechat.description")}</CardDescription>
        <CardAction>
          <div className="flex items-center gap-2">
            <Badge variant="outline">Official API</Badge>
            <WechatCredentialGuideDialog />
          </div>
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
                onAppIDChange(event.target.value);
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
                onAppSecretChange(event.target.value);
              }}
              placeholder={
                account?.has_app_secret
                  ? t("auth.wechat.saved")
                  : t("auth.wechat.placeholder")
              }
              disabled={loading || saving || testing}
            />
            <p className="text-xs leading-5 text-muted-foreground">
              {t("auth.wechat.secretHint")}
            </p>
          </div>

          <div className="flex flex-col gap-2 sm:flex-row">
            <Button
              type="button"
              onClick={onSave}
              disabled={loading || saving || testing || !canSubmit}
            >
              {saving ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Save className="size-4" />
              )}
              {t("auth.wechat.saveConfig")}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={onTest}
              disabled={loading || saving || testing || !canSubmit}
            >
              {testing ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <RefreshCw className="size-4" />
              )}
              {t("auth.wechat.testConnection")}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
