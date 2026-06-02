"use client";

import type { ComponentType, ReactNode } from "react";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

type DashboardStatCardProps = {
  description?: ReactNode;
  headerIcon?: ComponentType<{ className?: string }>;
  loading: boolean;
  skeletonClassName?: string;
  title: string;
  value: ReactNode;
  valueClassName?: string;
};

export function DashboardStatCard({
  description,
  headerIcon: HeaderIcon,
  loading,
  skeletonClassName = "h-8 w-24",
  title,
  value,
  valueClassName = "text-2xl font-bold",
}: DashboardStatCardProps) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
        {HeaderIcon ? (
          <HeaderIcon className="h-4 w-4 text-muted-foreground" />
        ) : null}
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className={skeletonClassName} />
        ) : (
          <div className={valueClassName}>{value}</div>
        )}
        {description ? (
          <p className="mt-1 text-xs text-muted-foreground">{description}</p>
        ) : null}
      </CardContent>
    </Card>
  );
}
