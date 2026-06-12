import { describe, expect, it } from "vitest";

import type { ProjectListItem, WorkspaceRole } from "@/lib/dashboard/api";

import { canDeleteProjectCard } from "./project-delete";

function project(overrides: Partial<ProjectListItem>): ProjectListItem {
  return {
    created_at: "2026-01-01T00:00:00.000Z",
    id: "project-1",
    publications: [],
    role: "editor",
    status: "ready",
    title: "Project",
    updated_at: "2026-01-02T00:00:00.000Z",
    user_id: "user-1",
    ...overrides,
  };
}

describe("project deletion policy", () => {
  it("allows owned projects in owner and workspace surfaces", () => {
    expect(
      canDeleteProjectCard(project({ access_source: "owner", role: "owner" }), {
        surface: "owned",
      }),
    ).toBe(true);
    expect(
      canDeleteProjectCard(project({ role: "owner" }), {
        surface: "workspace",
        workspaceRole: "member",
      }),
    ).toBe(true);
  });

  it("allows workspace owners and admins to delete workspace projects", () => {
    const roles: WorkspaceRole[] = ["owner", "admin"];

    expect(
      roles.map((workspaceRole) =>
        canDeleteProjectCard(project({ role: "editor" }), {
          surface: "workspace",
          workspaceRole,
        }),
      ),
    ).toEqual([true, true]);
  });

  it("blocks shared-with-me and non-admin workspace projects", () => {
    expect(
      canDeleteProjectCard(project({ role: "editor" }), {
        surface: "shared-with-me",
        workspaceRole: "admin",
      }),
    ).toBe(false);
    expect(
      canDeleteProjectCard(project({ role: "editor" }), {
        surface: "workspace",
        workspaceRole: "member",
      }),
    ).toBe(false);
  });
});
