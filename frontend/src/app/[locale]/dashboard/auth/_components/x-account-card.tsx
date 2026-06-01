"use client";

import { KeyRound, Loader2, RefreshCw, Save, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { type XAccount } from "@/lib/dashboard/api";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

type XAccountCardProps = {
  account: XAccount | null;
  accessToken: string;
  accessTokenSecret: string;
  apiKey: string;
  apiSecret: string;
  canSubmit: boolean;
  loading: boolean;
  saving: boolean;
  testing: boolean;
  username: string;
  onAccessTokenChange: (value: string) => void;
  onAccessTokenSecretChange: (value: string) => void;
  onAPIKeyChange: (value: string) => void;
  onAPISecretChange: (value: string) => void;
  onSave: () => void;
  onTest: () => void;
  onUsernameChange: (value: string) => void;
};

const xOAuth2AuthorizePath = "/api/user/dashboard/settings/x/oauth2/start";

export function XAccountCard({
  account,
  accessToken,
  accessTokenSecret,
  apiKey,
  apiSecret,
  canSubmit,
  loading,
  saving,
  testing,
  username,
  onAccessTokenChange,
  onAccessTokenSecretChange,
  onAPIKeyChange,
  onAPISecretChange,
  onSave,
  onTest,
  onUsernameChange,
}: XAccountCardProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");
  const disabled = loading || saving || testing;

  const handleOAuth2Authorize = () => {
    window.location.assign(xOAuth2AuthorizePath);
  };

  const formatExpiresAt = (value?: string) => {
    if (!value) {
      return t("auth.status.notAuthorized");
    }

    const date = new Date(value);
    if (Number.isNaN(date.valueOf())) {
      return t("auth.status.unknown");
    }

    return new Intl.DateTimeFormat(locale, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRound className="size-4" />
          {t("auth.x.title")}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">Official API</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="oauth2" className="w-full">
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="oauth2">OAuth 2.0</TabsTrigger>
            <TabsTrigger value="oauth1">OAuth 1.0a</TabsTrigger>
          </TabsList>

          <TabsContent value="oauth2" className="mt-4">
            <div className="flex flex-col gap-3 rounded-lg border bg-muted/30 p-4 sm:flex-row sm:items-center sm:justify-between">
              <Button
                type="button"
                onClick={handleOAuth2Authorize}
                disabled={disabled}
                className="w-full sm:w-auto"
              >
                <ShieldCheck className="size-4" />
                {t("auth.x.authorize")}
              </Button>
              <div className="text-sm text-muted-foreground">
                {t("auth.x.expiresAt")}
                <span className="font-medium text-foreground">
                  {formatExpiresAt(account?.expires_at)}
                </span>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="oauth1" className="mt-4">
            <div className="grid gap-5">
              <div className="grid gap-2 md:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor="x-api-key">API Key</Label>
                  <Input
                    id="x-api-key"
                    autoComplete="off"
                    value={apiKey}
                    onChange={(event) => {
                      onAPIKeyChange(event.target.value);
                    }}
                    placeholder="API Key"
                    disabled={disabled}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="x-username">Username</Label>
                  <Input
                    id="x-username"
                    autoComplete="off"
                    value={username}
                    onChange={(event) => {
                      onUsernameChange(event.target.value);
                    }}
                    placeholder="@username"
                    disabled={disabled}
                  />
                </div>
              </div>

              <div className="grid gap-2 md:grid-cols-3">
                <div className="grid gap-2">
                  <Label htmlFor="x-api-secret">API Secret</Label>
                  <Input
                    id="x-api-secret"
                    autoComplete="new-password"
                    type="password"
                    value={apiSecret}
                    onChange={(event) => {
                      onAPISecretChange(event.target.value);
                    }}
                    placeholder={
                      account?.has_api_secret
                        ? t("auth.x.savedHint")
                        : "API Secret"
                    }
                    disabled={disabled}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="x-access-token">Access Token</Label>
                  <Input
                    id="x-access-token"
                    autoComplete="new-password"
                    type="password"
                    value={accessToken}
                    onChange={(event) => {
                      onAccessTokenChange(event.target.value);
                    }}
                    placeholder={
                      account?.has_access_token
                        ? t("auth.x.savedHint")
                        : "Access Token"
                    }
                    disabled={disabled}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="x-access-token-secret">
                    Access Token Secret
                  </Label>
                  <Input
                    id="x-access-token-secret"
                    autoComplete="new-password"
                    type="password"
                    value={accessTokenSecret}
                    onChange={(event) => {
                      onAccessTokenSecretChange(event.target.value);
                    }}
                    placeholder={
                      account?.has_access_token_secret
                        ? t("auth.x.savedHint")
                        : "Access Token Secret"
                    }
                    disabled={disabled}
                  />
                </div>
              </div>

              <div className="flex flex-col gap-2 sm:flex-row">
                <Button
                  type="button"
                  onClick={onSave}
                  disabled={disabled || !canSubmit}
                >
                  {saving ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Save className="size-4" />
                  )}
                  {t("auth.actions.save")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={onTest}
                  disabled={disabled || !canSubmit}
                >
                  {testing ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <RefreshCw className="size-4" />
                  )}
                  {t("auth.actions.test")}
                </Button>
              </div>
            </div>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
