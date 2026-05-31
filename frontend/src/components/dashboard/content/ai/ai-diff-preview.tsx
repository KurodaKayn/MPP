"use client";

import { createTwoFilesPatch } from "diff";
import { useMemo } from "react";
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
  const file = useMemo(
    () => buildDiffFile(previousValue, nextValue),
    [nextValue, previousValue],
  );

  if (!file || file.hunks.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
        暂无差异
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

  const patch = createTwoFilesPatch(
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
  const files = parseDiff(patch, { nearbySequences: "zip" }) as FileData[];
  return files[0] ?? null;
}

function normalizeDiffText(value: string) {
  return value.endsWith("\n") ? value : `${value}\n`;
}
