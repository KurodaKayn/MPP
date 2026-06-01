"use client";

import { LogOut, Moon, Sun, Globe, Monitor } from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { useAuth } from "@/components/auth/auth-provider";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";

export function PreferencesCard() {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.preferences.title")}</CardTitle>
        <CardDescription>
          {t("settings.preferences.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-6">
        <div className="flex items-center justify-between space-x-2">
          <div className="flex items-center space-x-4">
            <Globe className="h-5 w-5 text-muted-foreground" />
            <div className="flex flex-col space-y-1">
              <Label>{t("settings.preferences.language")}</Label>
              <span className="text-[0.8rem] text-muted-foreground">
                {t("settings.preferences.languageDesc")}
              </span>
            </div>
          </div>
          {/* Mock Select for Language */}
          <div className="w-32 border rounded-md px-3 py-2 text-sm flex items-center justify-between opacity-50 cursor-not-allowed">
            <span>{t("settings.preferences.languageValue")}</span>
          </div>
        </div>

        <div className="flex items-center justify-between space-x-2">
          <div className="flex items-center space-x-4">
            <Moon className="h-5 w-5 text-muted-foreground" />
            <div className="flex flex-col space-y-1">
              <Label>{t("settings.preferences.theme")}</Label>
              <span className="text-[0.8rem] text-muted-foreground">
                {t("settings.preferences.themeDesc")}
              </span>
            </div>
          </div>
          <Tabs
            defaultValue="system"
            className="w-[200px] pointer-events-none opacity-50"
          >
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger value="light">
                <Sun className="h-4 w-4" />
              </TabsTrigger>
              <TabsTrigger value="dark">
                <Moon className="h-4 w-4" />
              </TabsTrigger>
              <TabsTrigger value="system">
                <Monitor className="h-4 w-4" />
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </CardContent>
    </Card>
  );
}

export function AccountManagementCard() {
  const router = useRouter();
  const { logout } = useAuth();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "dashboard");

  const handleLogout = () => {
    logout();
    toast.success(t("settings.account.logoutSuccess"));
    router.replace(`/${locale}/login`);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.account.title")}</CardTitle>
        <CardDescription>{t("settings.account.description")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-6">
        <div className="flex items-center justify-between space-x-2">
          <div className="flex items-center space-x-4">
            <LogOut className="h-5 w-5 text-muted-foreground" />
            <div className="flex flex-col space-y-1">
              <Label>{t("settings.account.logout")}</Label>
              <span className="text-[0.8rem] text-muted-foreground">
                {t("settings.account.logoutDesc")}
              </span>
            </div>
          </div>
          <Button variant="outline" onClick={handleLogout}>
            {t("settings.account.logoutButton")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
