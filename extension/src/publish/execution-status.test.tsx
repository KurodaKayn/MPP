import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ExtensionExecutionEvent } from "../types/events";
import type { ExtensionPublishHandoff } from "../types/handoff";
import { HANDOFF_SCHEMA_VERSION, HANDOFF_TYPE } from "../types/handoff";
import { CompactExecutionStatus } from "./execution-status";

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
      {
        platform: "bilibili",
        adapter_key: "DYNAMIC_BILIBILI",
        inject_url: "https://member.bilibili.com/",
        content_kind: "dynamic_post",
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

function createEvent(
  status: ExtensionExecutionEvent["status"],
  platform: ExtensionExecutionEvent["platform"],
): ExtensionExecutionEvent {
  return {
    event_id: `${platform}-${status}`,
    platform,
    status,
    message: `${platform} ${status}`,
    remote_id: "",
    publish_url: "",
    error_message: "",
    metadata: {},
    created_at: "2026-06-03T12:00:00Z",
  };
}

describe("CompactExecutionStatus", () => {
  it("does not show execution metrics before a handoff is active", () => {
    const { container } = render(
      <CompactExecutionStatus handoff={null} events={[]} />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it("summarizes active platform progress with user-facing copy", () => {
    render(
      <CompactExecutionStatus
        handoff={createHandoff()}
        events={[
          createEvent("injecting", "douyin"),
          createEvent("user_review", "douyin"),
        ]}
      />,
    );

    expect(screen.getByText("Platforms ready")).toBeInTheDocument();
    expect(screen.getByText("1/2")).toBeInTheDocument();
    expect(screen.getByText("Latest update")).toBeInTheDocument();
    expect(
      screen.getByText("Draft ready for review in Douyin."),
    ).toBeInTheDocument();
    expect(screen.queryByText("user review")).not.toBeInTheDocument();
    expect(screen.queryByText("Execution Events")).not.toBeInTheDocument();
  });
});
