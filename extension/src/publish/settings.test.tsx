import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ExtensionExecutionEvent } from "../types/events";
import type { ExtensionPublishHandoff } from "../types/handoff";
import { HANDOFF_SCHEMA_VERSION, HANDOFF_TYPE } from "../types/handoff";
import { AccountSettings, DiagnosticsSettings } from "./settings";

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

describe("AccountSettings", () => {
  it("groups session state and actions under account settings", () => {
    render(
      <AccountSettings
        state={{
          status: "authenticated",
          user: {
            id: "user-1",
            username: "creator",
          },
        }}
        onOpenLogin={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByText("Account Settings")).toBeInTheDocument();
    expect(screen.getByText("creator")).toBeInTheDocument();
    expect(screen.getByText("connected")).toBeInTheDocument();
  });
});

describe("DiagnosticsSettings", () => {
  it("keeps version, origins, platform details, and events in diagnostics", () => {
    const removeOrigin = vi.fn();

    render(
      <DiagnosticsSettings
        version="0.0.1"
        handoff={createHandoff()}
        events={[createEvent()]}
        trustedOrigins={[
          {
            origin: "https://mpp.example.com",
            trusted_at: "2026-06-03T12:00:00Z",
          },
        ]}
        onReopen={vi.fn()}
        onRemoveOrigin={removeOrigin}
      />,
    );

    expect(screen.getByText("Diagnostics Settings")).toBeInTheDocument();
    expect(screen.getByText("Extension Version")).toBeInTheDocument();
    expect(screen.getByText("v0.0.1")).toBeInTheDocument();
    expect(screen.getByText("Active Execution")).toBeInTheDocument();
    expect(screen.getAllByText("Platforms").length).toBeGreaterThan(0);
    expect(screen.getByText("Execution Events")).toBeInTheDocument();
    expect(screen.getByText("Trusted Origins")).toBeInTheDocument();
    expect(screen.getByText("https://mpp.example.com")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", {
        name: /remove trusted origin https:\/\/mpp.example.com/i,
      }),
    );

    expect(removeOrigin).toHaveBeenCalledWith("https://mpp.example.com");
  });
});
