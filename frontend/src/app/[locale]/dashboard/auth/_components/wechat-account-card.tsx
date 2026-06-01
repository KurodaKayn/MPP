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
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRound className="size-4" />
          微信公众号
        </CardTitle>
        <CardDescription>使用公众平台开发者凭证连接官方接口。</CardDescription>
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
              onClick={onSave}
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
              onClick={onTest}
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
  );
}
