"use client";

import { AlertCircle, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

type DashboardErrorCardProps = {
  compact?: boolean;
  message: string;
  onRetry?: () => void;
  retryDisabled?: boolean;
  retryLabel?: string;
  title: string;
};

export function DashboardErrorCard({
  compact = false,
  message,
  onRetry,
  retryDisabled = false,
  retryLabel,
  title,
}: DashboardErrorCardProps) {
  return (
    <Card className="border-destructive/40 bg-destructive/5">
      <CardContent
        className={
          compact
            ? "flex items-start gap-3 py-4"
            : "flex flex-col gap-3 py-4 sm:flex-row sm:items-center sm:justify-between"
        }
      >
        <div className="flex items-start gap-3">
          <AlertCircle className="mt-0.5 h-4 w-4 text-destructive" />
          <div>
            <div className="text-sm font-medium">{title}</div>
            <p className="text-sm text-muted-foreground">{message}</p>
          </div>
        </div>
        {retryLabel && onRetry ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onRetry}
            disabled={retryDisabled}
          >
            <RefreshCw className="h-4 w-4" />
            {retryLabel}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}
