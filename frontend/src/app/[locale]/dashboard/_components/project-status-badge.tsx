import type { ComponentProps } from "react";

import { Badge } from "@/components/ui/badge";

const statusVariants: Record<string, ComponentProps<typeof Badge>["variant"]> =
  {
    adapted: "secondary",
    disabled: "outline",
    draft: "outline",
    failed: "destructive",
    pending: "outline",
    published: "default",
    publishing: "secondary",
    queued: "secondary",
    ready: "secondary",
    succeeded: "default",
    syncing: "secondary",
    cancelled: "outline",
  };

type ProjectStatusBadgeProps = {
  label: string;
  status: string;
};

export function ProjectStatusBadge({ label, status }: ProjectStatusBadgeProps) {
  return <Badge variant={statusVariants[status] ?? "outline"}>{label}</Badge>;
}
