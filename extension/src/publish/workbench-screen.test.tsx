import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ExtensionPrepublishResponse } from "../backend/types";
import type { ExtensionExecutionEvent } from "../types/events";
import type { ExtensionPublishHandoff } from "../types/handoff";
import { HANDOFF_SCHEMA_VERSION, HANDOFF_TYPE } from "../types/handoff";
import type { PrepublishWorkbenchProps } from "./prepublish";
import type { SessionViewState } from "./session";
import { PublishWorkbenchScreen } from "./workbench-screen";

function createPrepublishResponse(): ExtensionPrepublishResponse {
  return {
    items: [
      {
        project_id: "project-1",
        title: "Douyin article draft",
        status: "ready",
        updated_at: "2026-06-03T10:00:00Z",
        platforms: [
          {
            publication_id: "publication-1",
            platform: "douyin",
            adapter_key: "DYNAMIC_DOUYIN",
            content_kind: "article",
            status: "adapted",
            enabled: true,
            preview: "First draft preview",
          },
        ],
      },
    ],
  };
}

function createWorkbenchProps(): PrepublishWorkbenchProps {
  return {
    state: {
      status: "loaded",
      items: createPrepublishResponse().items,
    },
    selectedProjectId: "project-1",
    selectedPlatforms: new Set(["douyin"]),
    onProjectSelect: vi.fn(),
    onPlatformToggle: vi.fn(),
    onRetry: vi.fn(),
  };
}

function createSessionState(): SessionViewState {
  return {
    status: "authenticated",
    user: {
      id: "user-1",
      username: "creator",
    },
  };
}

function createHandoff(): ExtensionPublishHandoff {
  return {
    schema_version: HANDOFF_SCHEMA_VERSION,
    type: HANDOFF_TYPE,
    execution_id: "execution-1",
    expires_at: "2026-06-03T12:00:00Z",
    project: {
      id: "project-1",
      title: "Douyin article draft",
    },
    platforms: [
      {
        platform: "douyin",
        adapter_key: "DYNAMIC_DOUYIN",
        inject_url: "https://creator.douyin.com/",
        content_kind: "article",
        auto_publish: false,
        requires_review: true,
        adapted_content: {
          schema_version: HANDOFF_SCHEMA_VERSION,
          format: "markdown",
          markdown: "draft body",
        },
        assets: [],
      },
    ],
  };
}

function createEvent(): ExtensionExecutionEvent {
  return {
    event_id: "event-1",
    platform: "douyin",
    status: "user_review",
    message: "Draft ready for review.",
    remote_id: "",
    publish_url: "",
    error_message: "",
    metadata: {},
    created_at: "2026-06-03T12:00:00Z",
  };
}

describe("PublishWorkbenchScreen", () => {
  it("renders the default workbench without account or diagnostics details", () => {
    render(
      <PublishWorkbenchScreen
        error=""
        version="0.0.1"
        handoff={null}
        events={[]}
        prepublishWorkbench={createWorkbenchProps()}
        startingHandoff={false}
        handoffStartError=""
        sessionState={createSessionState()}
        trustedOrigins={[]}
        settingsLoading={false}
        onRefresh={vi.fn()}
        onOpenLogin={vi.fn()}
        onRefreshSession={vi.fn()}
        onStartHandoff={vi.fn()}
        onReopenPlatform={vi.fn()}
        onRemoveOrigin={vi.fn()}
        onClearExecutionState={vi.fn()}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "MPP Publisher" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Pre-Publish Drafts")).toBeInTheDocument();
    expect(screen.getAllByText("Douyin article draft").length).toBeGreaterThan(
      0,
    );

    expect(screen.queryByText("v0.0.1")).not.toBeInTheDocument();
    expect(screen.queryByText("Account Settings")).not.toBeInTheDocument();
    expect(screen.queryByText("Diagnostics Settings")).not.toBeInTheDocument();
    expect(screen.queryByText("Extension Version")).not.toBeInTheDocument();
    expect(screen.queryByText("Trusted Origins")).not.toBeInTheDocument();
    expect(screen.queryByText("Execution Events")).not.toBeInTheDocument();
    expect(screen.queryByText("Active Execution")).not.toBeInTheDocument();
  });

  it("opens settings with account and expandable diagnostics controls", () => {
    const refreshSession = vi.fn();
    const reopenPlatform = vi.fn();
    const removeOrigin = vi.fn();
    const clearExecutionState = vi.fn();

    render(
      <PublishWorkbenchScreen
        error=""
        version="0.0.1"
        handoff={createHandoff()}
        events={[createEvent()]}
        prepublishWorkbench={createWorkbenchProps()}
        startingHandoff={false}
        handoffStartError=""
        sessionState={createSessionState()}
        trustedOrigins={[
          {
            origin: "https://mpp.example.com",
            trusted_at: "2026-06-03T12:00:00Z",
          },
        ]}
        settingsLoading={false}
        onRefresh={vi.fn()}
        onOpenLogin={vi.fn()}
        onRefreshSession={refreshSession}
        onStartHandoff={vi.fn()}
        onReopenPlatform={reopenPlatform}
        onRemoveOrigin={removeOrigin}
        onClearExecutionState={clearExecutionState}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /open settings/i }));

    expect(
      screen.getByRole("dialog", { name: "Settings" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Account Settings")).toBeInTheDocument();
    expect(screen.getByText("creator")).toBeInTheDocument();
    expect(screen.getByText("connected")).toBeInTheDocument();
    expect(screen.queryByText("Trusted Origins")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /refresh session/i }));
    expect(refreshSession).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole("button", { name: /show diagnostics/i }));

    expect(screen.getByText("Diagnostics Settings")).toBeInTheDocument();
    expect(screen.getByText("Extension Version")).toBeInTheDocument();
    expect(screen.getByText("v0.0.1")).toBeInTheDocument();
    expect(screen.getByText("Trusted Origins")).toBeInTheDocument();
    expect(screen.getByText("Execution Events")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", {
        name: /remove trusted origin https:\/\/mpp.example.com/i,
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: /clear execution/i }));
    fireEvent.click(screen.getByRole("button", { name: /reopen/i }));

    expect(removeOrigin).toHaveBeenCalledWith("https://mpp.example.com");
    expect(clearExecutionState).toHaveBeenCalledOnce();
    expect(reopenPlatform).toHaveBeenCalledWith(
      expect.objectContaining({ platform: "douyin" }),
    );
  });
});
