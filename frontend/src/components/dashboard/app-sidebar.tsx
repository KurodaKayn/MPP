"use client";

import * as React from "react";
import {
  LayoutDashboard,
  FileText,
  Settings,
  PlusCircle,
  Key,
} from "lucide-react";

import Image from "next/image";
import Link from "next/link";

import { useAuth } from "@/components/auth/auth-provider";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
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
import { dashboardRoutes } from "@/lib/dashboard/navigation";

const data = {
  navMain: [
    {
      ...dashboardRoutes.overview,
      icon: LayoutDashboard,
    },
    {
      ...dashboardRoutes.content,
      icon: PlusCircle,
    },
    {
      ...dashboardRoutes.posts,
      icon: FileText,
    },
    {
      ...dashboardRoutes.auth,
      icon: Key,
    },
  ],
};

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const { session } = useAuth();
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const username = session?.username ?? t("nav.creator");
  const initials = username.slice(0, 2).toUpperCase();

  const getLocalizedUrl = (url: string) => `/${locale}${url}`;

  return (
    <Sidebar collapsible="icon" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={(buttonProps) => (
                <Link
                  href={getLocalizedUrl(dashboardRoutes.overview.url)}
                  {...buttonProps}
                >
                  <div className="flex items-center">
                    <Image
                      src="/icons/mpp-with-name.svg"
                      alt="Multi-Poster"
                      width={140}
                      height={38}
                      className="h-9 w-auto"
                    />
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
                tooltip={t(item.title)}
                render={(buttonProps) => (
                  <Link href={getLocalizedUrl(item.url)} {...buttonProps}>
                    <item.icon />
                    <span>{t(item.title)}</span>
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
              tooltip={t("nav.settings")}
              render={(buttonProps) => (
                <Link
                  href={getLocalizedUrl(dashboardRoutes.settings.url)}
                  {...buttonProps}
                >
                  <Avatar className="h-8 w-8 rounded-lg">
                    <AvatarFallback className="rounded-lg">
                      {initials}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid min-w-0 flex-1 text-left text-sm leading-tight">
                    <span className="truncate font-semibold">{username}</span>
                    <span className="truncate text-xs">
                      {t("nav.loggedIn")}
                    </span>
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
