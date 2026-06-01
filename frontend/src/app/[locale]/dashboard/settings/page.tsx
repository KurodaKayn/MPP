import { SettingsPageHeader } from "./_components/settings-page-header";
import {
  PreferencesCard,
  AccountManagementCard,
} from "./_components/preferences-card";

export default function SettingsPage() {
  return (
    <div className="mx-auto w-full flex max-w-6xl flex-col gap-4">
      <SettingsPageHeader />

      <PreferencesCard />
      <AccountManagementCard />
    </div>
  );
}
