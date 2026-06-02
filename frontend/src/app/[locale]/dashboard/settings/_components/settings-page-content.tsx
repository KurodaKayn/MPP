import { AccountManagementCard, PreferencesCard } from "./preferences-card";
import { SettingsPageHeader } from "./settings-page-header";

export function SettingsPageContent() {
  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4">
      <SettingsPageHeader />
      <PreferencesCard />
      <AccountManagementCard />
    </div>
  );
}
