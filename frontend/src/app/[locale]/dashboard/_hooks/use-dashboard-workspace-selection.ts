"use client";

import {
  createContext,
  createElement,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  createWorkspace as createWorkspaceRequest,
  getWorkspaces,
  type CreateWorkspaceInput,
  type Workspace,
  type WorkspaceRole,
} from "@/lib/dashboard/api";

type UseDashboardWorkspaceSelectionOptions = {
  enabled?: boolean;
};

type DashboardWorkspaceSelection = {
  createWorkspace: (input: CreateWorkspaceInput) => Promise<Workspace>;
  error: string;
  isCreatingWorkspace: boolean;
  isLoading: boolean;
  reloadWorkspaces: () => Promise<void>;
  selectedWorkspace: Workspace | null;
  selectedWorkspaceId: string;
  selectWorkspace: (workspaceId: string) => void;
  workspaces: Workspace[];
};

const DashboardWorkspaceSelectionContext =
  createContext<DashboardWorkspaceSelection | null>(null);

const selectedWorkspaceStorageKey = "mpp.dashboard.selectedWorkspaceId";

export function canCreateWorkspaceProject(role?: WorkspaceRole | null) {
  return role === "owner" || role === "admin" || role === "member";
}

function readStoredWorkspaceId() {
  if (typeof window === "undefined") {
    return "";
  }

  return window.localStorage.getItem(selectedWorkspaceStorageKey) ?? "";
}

function useDashboardWorkspaceSelectionState({
  enabled = true,
}: UseDashboardWorkspaceSelectionOptions = {}): DashboardWorkspaceSelection {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState("");
  const [isLoading, setIsLoading] = useState(enabled);
  const [isCreatingWorkspace, setIsCreatingWorkspace] = useState(false);
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
        const preferredWorkspaceId =
          currentWorkspaceId || readStoredWorkspaceId();
        if (
          preferredWorkspaceId &&
          response.items.some(
            (workspace) => workspace.id === preferredWorkspaceId,
          )
        ) {
          return preferredWorkspaceId;
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

  useEffect(() => {
    if (!enabled || isLoading || typeof window === "undefined") {
      return;
    }

    if (selectedWorkspaceId) {
      window.localStorage.setItem(
        selectedWorkspaceStorageKey,
        selectedWorkspaceId,
      );
    } else {
      window.localStorage.removeItem(selectedWorkspaceStorageKey);
    }
  }, [enabled, isLoading, selectedWorkspaceId]);

  const createWorkspace = useCallback(async (input: CreateWorkspaceInput) => {
    setIsCreatingWorkspace(true);
    try {
      const workspace = await createWorkspaceRequest(input);
      setWorkspaces((items) => [
        workspace,
        ...items.filter((item) => item.id !== workspace.id),
      ]);
      setSelectedWorkspaceId(workspace.id);
      setError("");
      return workspace;
    } finally {
      setIsCreatingWorkspace(false);
    }
  }, []);

  const selectedWorkspace = useMemo(
    () =>
      workspaces.find((workspace) => workspace.id === selectedWorkspaceId) ??
      null,
    [selectedWorkspaceId, workspaces],
  );

  return {
    createWorkspace,
    error,
    isCreatingWorkspace,
    isLoading,
    reloadWorkspaces: loadWorkspaces,
    selectedWorkspace,
    selectedWorkspaceId,
    selectWorkspace: setSelectedWorkspaceId,
    workspaces,
  };
}

function disabledWorkspaceSelection(
  createWorkspace: DashboardWorkspaceSelection["createWorkspace"],
): DashboardWorkspaceSelection {
  return {
    createWorkspace,
    error: "",
    isCreatingWorkspace: false,
    isLoading: false,
    reloadWorkspaces: async () => {},
    selectedWorkspace: null,
    selectedWorkspaceId: "",
    selectWorkspace: () => {},
    workspaces: [],
  };
}

export function DashboardWorkspaceProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const value = useDashboardWorkspaceSelectionState();

  return createElement(
    DashboardWorkspaceSelectionContext.Provider,
    { value },
    children,
  );
}

export function useDashboardWorkspaceSelection({
  enabled = true,
}: UseDashboardWorkspaceSelectionOptions = {}) {
  const context = useContext(DashboardWorkspaceSelectionContext);
  const localSelection = useDashboardWorkspaceSelectionState({
    enabled: context ? false : enabled,
  });

  if (!context) {
    return localSelection;
  }

  if (!enabled) {
    return disabledWorkspaceSelection(context.createWorkspace);
  }

  return context;
}
