"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  getWorkspaces,
  type Workspace,
  type WorkspaceRole,
} from "@/lib/dashboard/api";

type UseDashboardWorkspaceSelectionOptions = {
  enabled?: boolean;
};

export function canCreateWorkspaceProject(role?: WorkspaceRole | null) {
  return role === "owner" || role === "admin" || role === "member";
}

export function useDashboardWorkspaceSelection({
  enabled = true,
}: UseDashboardWorkspaceSelectionOptions = {}) {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState("");
  const [isLoading, setIsLoading] = useState(enabled);
  const [error, setError] = useState("");

  const loadWorkspaces = useCallback(async () => {
    if (!enabled) {
      return;
    }

    setIsLoading(true);
    setError("");

    try {
      const response = await getWorkspaces();
      setWorkspaces(response.items);
      setSelectedWorkspaceId((currentWorkspaceId) => {
        if (
          currentWorkspaceId &&
          response.items.some(
            (workspace) => workspace.id === currentWorkspaceId,
          )
        ) {
          return currentWorkspaceId;
        }

        return response.items[0]?.id ?? "";
      });
    } catch (requestError) {
      setWorkspaces([]);
      setSelectedWorkspaceId("");
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Unable to load workspaces",
      );
    } finally {
      setIsLoading(false);
    }
  }, [enabled]);

  useEffect(() => {
    if (!enabled) {
      setIsLoading(false);
      return;
    }

    void loadWorkspaces();
  }, [enabled, loadWorkspaces]);

  const selectedWorkspace = useMemo(
    () =>
      workspaces.find((workspace) => workspace.id === selectedWorkspaceId) ??
      null,
    [selectedWorkspaceId, workspaces],
  );

  return {
    error,
    isLoading,
    reloadWorkspaces: loadWorkspaces,
    selectedWorkspace,
    selectedWorkspaceId,
    selectWorkspace: setSelectedWorkspaceId,
    workspaces,
  };
}
