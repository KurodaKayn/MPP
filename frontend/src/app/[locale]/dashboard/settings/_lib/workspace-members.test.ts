import { describe, expect, it } from "vitest";
import {
  canManageWorkspaceMembers,
  manageableWorkspaceMemberRoles,
} from "./workspace-members";

describe("workspace member management policy", () => {
  it("allows owners and admins to manage workspace members", () => {
    expect(canManageWorkspaceMembers("owner")).toBe(true);
    expect(canManageWorkspaceMembers("admin")).toBe(true);
  });

  it("blocks non-manager roles and missing workspace roles", () => {
    expect(canManageWorkspaceMembers("member")).toBe(false);
    expect(canManageWorkspaceMembers("viewer")).toBe(false);
    expect(canManageWorkspaceMembers(null)).toBe(false);
    expect(canManageWorkspaceMembers(undefined)).toBe(false);
  });

  it("offers only assignable member roles", () => {
    expect(manageableWorkspaceMemberRoles).toEqual([
      "admin",
      "member",
      "viewer",
    ]);
    expect(manageableWorkspaceMemberRoles).not.toContain("owner");
  });
});
