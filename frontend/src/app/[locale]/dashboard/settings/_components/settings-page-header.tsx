"use client";

export function SettingsPageHeader() {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">设置</h1>
        <p className="text-muted-foreground mt-1">
          管理您的偏好设置和账号安全。
        </p>
      </div>
    </div>
  );
}
