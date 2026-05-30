"use client";

import * as React from "react";
import { LayoutDashboard, FileText, Settings, PlusCircle } from "lucide-react";
import Image from "next/image";
import Link from "next/link";

import { useAuth } from "@/components/auth/auth-provider";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";

const data = {
  navMain: [
    {
      title: "概览",
      url: "/dashboard",
      icon: LayoutDashboard,
    },
    {
      title: "内容创作",
      url: "/dashboard/content",
      icon: PlusCircle,
    },
    {
      title: "我的内容",
      url: "/dashboard/posts",
      icon: FileText,
    },
  ],
};

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { session } = useAuth();
  const username = session?.username ?? "Creator";
  const initials = username.slice(0, 2).toUpperCase();

  return (
    <Sidebar collapsible="icon" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={(buttonProps) => (
                <Link href="/dashboard" {...buttonProps}>
                  <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                    <Image
                      src="/icons/mpp.svg"
                      alt="Multi-Poster"
                      width={20}
                      height={20}
                      className="size-5"
                    />
                  </div>
                  <div className="grid flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-semibold">Multi-Poster</span>
                    <span className="truncate text-xs">内容一键分发</span>
                  </div>
                </Link>
              )}
            />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarMenu>
          {data.navMain.map((item) => (
            <SidebarMenuItem key={item.title}>
              <SidebarMenuButton
                tooltip={item.title}
                render={(buttonProps) => (
                  <Link href={item.url} {...buttonProps}>
                    <item.icon />
                    <span>{item.title}</span>
                  </Link>
                )}
              />
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              tooltip="设置"
              render={(buttonProps) => (
                <Link href="/dashboard/settings" {...buttonProps}>
                  <Avatar className="h-8 w-8 rounded-lg">
                    <AvatarFallback className="rounded-lg">
                      {initials}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid min-w-0 flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-semibold">{username}</span>
                    <span className="truncate text-xs">已登录</span>
                  </div>
                  <Settings className="ml-auto size-4 text-sidebar-foreground/70" />
                </Link>
              )}
            />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
