import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ExtensionPrepublishResponse } from "../backend/types";
import type { PrepublishWorkbenchProps } from "./prepublish";
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
        onRefresh={vi.fn()}
        onOpenLogin={vi.fn()}
        onStartHandoff={vi.fn()}
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
});
