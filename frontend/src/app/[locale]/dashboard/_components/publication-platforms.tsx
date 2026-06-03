import Image from "next/image";

import { Badge } from "@/components/ui/badge";
import { PLATFORM_TABS } from "@/lib/content/platforms";

import type { PublicationSummary } from "../_lib/publications";

function getPlatformMetadata(platform: string) {
  return PLATFORM_TABS.find((item) => item.value === platform);
}

function getPlatformLabel(platform: string, tCommon: any) {
  const metadata = getPlatformMetadata(platform);
  return metadata ? tCommon(metadata.label) : platform;
}

export function PublicationBadgeList({
  emptyLabel,
  publications,
  tCommon,
}: {
  emptyLabel: string;
  publications: PublicationSummary[];
  tCommon: any;
}) {
  if (publications.length === 0) {
    return <span className="text-muted-foreground">{emptyLabel}</span>;
  }

  return (
    <div className="flex flex-wrap gap-1.5">
      {publications.map((publication) => (
        <Badge
          key={publication.id}
          variant={publication.status === "failed" ? "destructive" : "outline"}
        >
          {getPlatformLabel(publication.platform, tCommon)}
        </Badge>
      ))}
    </div>
  );
}

export function PlatformIcon({
  platform,
  tCommon,
}: {
  platform: string;
  tCommon: any;
}) {
  const metadata = getPlatformMetadata(platform);

  if (!metadata) {
    return (
      <span
        aria-label={platform}
        className="flex size-7 items-center justify-center rounded-md border bg-muted text-[10px] font-semibold uppercase text-muted-foreground"
      >
        {platform.slice(0, 2)}
      </span>
    );
  }

  return (
    <span
      className="flex size-7 items-center justify-center rounded-md border bg-background"
      title={tCommon(metadata.label)}
    >
      <Image
        src={metadata.icon}
        alt={tCommon(metadata.label)}
        width={18}
        height={18}
        className="size-[18px]"
      />
    </span>
  );
}

export function PlatformIconRow({
  emptyLabel,
  label,
  publications,
  tCommon,
}: {
  emptyLabel: string;
  label: string;
  publications: PublicationSummary[];
  tCommon: any;
}) {
  return (
    <div className="grid grid-cols-[4.75rem_minmax(0,1fr)] items-center gap-3 text-sm">
      <div className="whitespace-nowrap text-muted-foreground">{label}:</div>
      <div className="flex min-h-7 flex-wrap items-center gap-2">
        {publications.length > 0 ? (
          publications.map((publication) => (
            <PlatformIcon
              key={`${publication.id}-${publication.platform}`}
              platform={publication.platform}
              tCommon={tCommon}
            />
          ))
        ) : (
          <span className="text-xs text-muted-foreground">{emptyLabel}</span>
        )}
      </div>
    </div>
  );
}
