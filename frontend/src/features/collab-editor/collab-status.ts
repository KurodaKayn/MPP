import { cn } from "@/lib/utils";
import type { CollabConnectionStatus } from "./collab-provider";

export function collabStatusClassName(status: CollabConnectionStatus) {
  return cn(
    status === "synced" &&
      "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
    status === "connected" &&
      "border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300",
    (status === "offline" || status === "error" || status === "unauthorized") &&
      "border-destructive/30 bg-destructive/10 text-destructive",
  );
}
