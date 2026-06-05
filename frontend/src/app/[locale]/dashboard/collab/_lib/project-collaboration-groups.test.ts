import { describe, expect, it } from "vitest";

import type { ProjectCollaborator, ProjectListItem } from "@/lib/dashboard/api";

import {
  getOwnedProjects,
  getProjectsSharedByMe,
  getProjectsSharedWithMe,
} from "./project-collaboration-groups";

function project(overrides: Partial<ProjectListItem>): ProjectListItem {
  return {
    created_at: "2026-01-01T00:00:00.000Z",
    id: "project-1",
    publications: [],
    role: "owner",
    status: "ready",
    title: "Project",
    updated_at: "2026-01-02T00:00:00.000Z",
    user_id: "user-1",
    ...overrides,
  };
}

function collaborator(
  overrides: Partial<ProjectCollaborator>,
): ProjectCollaborator {
  return {
    created_at: "2026-01-03T00:00:00.000Z",
    created_by: "user-1",
    email: "teammate@example.com",
    project_id: "project-1",
    role: "editor",
    user_id: "user-2",
    username: "teammate",
    ...overrides,
  };
}

describe("project collaboration groups", () => {
  it("uses access source to identify owned projects", () => {
    const projects = [
      project({ id: "owned", access_source: "owner", role: "owner" }),
      project({ id: "shared", access_source: "direct_share", role: "editor" }),
      project({ id: "workspace", access_source: "workspace", role: "editor" }),
      project({ id: "legacy-owned", role: "owner" }),
    ];

    expect(getOwnedProjects(projects).map((item) => item.id)).toEqual([
      "owned",
      "legacy-owned",
    ]);
  });

  it("keeps direct shares separate from workspace projects", () => {
    const projects = [
      project({ id: "owned", access_source: "owner", role: "owner" }),
      project({ id: "direct", access_source: "direct_share", role: "viewer" }),
      project({ id: "workspace", access_source: "workspace", role: "editor" }),
      project({ id: "legacy-direct", role: "viewer" }),
      project({
        id: "legacy-workspace",
        role: "editor",
        workspace_id: "workspace-1",
      }),
    ];

    expect(getProjectsSharedWithMe(projects).map((item) => item.id)).toEqual([
      "direct",
      "legacy-direct",
    ]);
  });

  it("returns only owned projects with collaborators as shared by me", () => {
    const first = project({ id: "first" });
    const second = project({ id: "second" });

    expect(
      getProjectsSharedByMe([
        {
          collaborators: [
            collaborator({ project_id: first.id, user_id: "user-2" }),
            collaborator({ project_id: first.id, user_id: "user-3" }),
          ],
          project: first,
        },
        {
          collaborators: [],
          project: second,
        },
      ]),
    ).toEqual([
      {
        collaboratorCount: 2,
        collaborators: [
          collaborator({ project_id: first.id, user_id: "user-2" }),
          collaborator({ project_id: first.id, user_id: "user-3" }),
        ],
        project: first,
      },
    ]);
  });
});
