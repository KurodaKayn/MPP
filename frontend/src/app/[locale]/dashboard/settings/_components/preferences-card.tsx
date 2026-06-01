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

export function PreferencesCard() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>通用设置</CardTitle>
        <CardDescription>配置界面的语言和主题风格。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-6">
        <div className="flex items-center justify-between space-x-2">
          <div className="flex items-center space-x-4">
            <Globe className="h-5 w-5 text-muted-foreground" />
            <div className="flex flex-col space-y-1">
              <Label>语言</Label>
              <span className="text-[0.8rem] text-muted-foreground">
                选择您偏好的界面语言。
              </span>
            </div>
          </div>
          {/* Mock Select for Language */}
          <div className="w-32 border rounded-md px-3 py-2 text-sm flex items-center justify-between opacity-50 cursor-not-allowed">
            <span>简体中文</span>
          </div>
        </div>

        <div className="flex items-center justify-between space-x-2">
          <div className="flex items-center space-x-4">
            <Moon className="h-5 w-5 text-muted-foreground" />
            <div className="flex flex-col space-y-1">
              <Label>深色模式</Label>
              <span className="text-[0.8rem] text-muted-foreground">
                当前：跟随系统
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

export function DangerZoneCard() {
  const router = useRouter();
  const { logout } = useAuth();

  const handleLogout = () => {
    logout();
    toast.success("已退出登录");
    router.replace("/login");
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>账号管理</CardTitle>
        <CardDescription>管理您的登录状态。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-6">
        <div className="flex items-center justify-between space-x-2">
          <div className="flex items-center space-x-4">
            <LogOut className="h-5 w-5 text-muted-foreground" />
            <div className="flex flex-col space-y-1">
              <Label>退出账号</Label>
              <span className="text-[0.8rem] text-muted-foreground">
                在此设备上退出当前的管理员账号。
              </span>
            </div>
          </div>
          <Button variant="outline" onClick={handleLogout}>
            退出登录
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
