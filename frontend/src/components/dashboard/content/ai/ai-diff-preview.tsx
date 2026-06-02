"use client";

import { createTwoFilesPatch } from "diff";
import { useMemo } from "react";
import { useAppLocale, useTranslation } from "@/lib/i18n/client";
import {
  Diff,
  Hunk,
  parseDiff,
  type DiffType,
  type FileData,
} from "react-diff-view";

type AIDiffPreviewProps = {
  nextValue: string;
  previousValue: string;
};

export function AIDiffPreview({
  nextValue,
  previousValue,
}: AIDiffPreviewProps) {
  const locale = useAppLocale();
  const { t } = useTranslation(locale, "common");
  const file = useMemo(
    () => buildDiffFile(previousValue, nextValue),
    [nextValue, previousValue],
  );

  if (!file || file.hunks.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
        {t("ai.noDiff")}
      </div>
    );
  }

  return (
    <div className="ai-diff-view overflow-x-auto rounded-md border bg-background text-xs">
      <Diff
        diffType={file.type as DiffType}
        hunks={file.hunks}
        viewType="split"
      >
        {(hunks) =>
          hunks.map((hunk) => <Hunk key={hunk.content} hunk={hunk} />)
        }
      </Diff>
    </div>
  );
}

function buildDiffFile(previousValue: string, nextValue: string) {
  if (previousValue === nextValue) {
    return null;
  }

  const patch = createGitPatch(
    "before.md",
    "after.md",
    normalizeDiffText(previousValue),
    normalizeDiffText(nextValue),
    "before",
    "after",
    {
      context: 4,
    },
  );
  try {
    const files = parseDiff(patch, { nearbySequences: "zip" }) as FileData[];
    return files[0] ?? null;
  } catch {
    return null;
  }
}

function createGitPatch(
  previousFileName: string,
  nextFileName: string,
  previousValue: string,
  nextValue: string,
  previousHeader: string,
  nextHeader: string,
  options: { context: number },
) {
  const patch = createTwoFilesPatch(
    previousFileName,
    nextFileName,
    previousValue,
    nextValue,
    previousHeader,
    nextHeader,
    options,
  );
  const body = patch
    .replace(/^=+\n/, "")
    .replace(
      new RegExp(`^--- ${escapeRegExp(previousFileName)}\\t.*$`, "m"),
      `--- a/${previousFileName}`,
    )
    .replace(
      new RegExp(`^\\+\\+\\+ ${escapeRegExp(nextFileName)}\\t.*$`, "m"),
      `+++ b/${nextFileName}`,
    );

  return [
    `diff --git a/${previousFileName} b/${nextFileName}`,
    "index 0000000..1111111 100644",
    body,
  ].join("\n");
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function normalizeDiffText(value: string) {
  return value.endsWith("\n") ? value : `${value}\n`;
}
