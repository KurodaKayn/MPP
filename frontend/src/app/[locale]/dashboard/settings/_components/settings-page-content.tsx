import { AccountManagementCard, PreferencesCard } from "./preferences-card";
import { SettingsPageHeader } from "./settings-page-header";
import { WorkspaceActivityCard } from "./workspace-activity-card";
import { WorkspaceMembersCard } from "./workspace-members-card";

export function SettingsPageContent() {
  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4">
      <SettingsPageHeader />
      <WorkspaceMembersCard />
      <WorkspaceActivityCard />
      <PreferencesCard />
      <AccountManagementCard />
    </div>
  );
}
