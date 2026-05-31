"use client";

import ReactMarkdown from "react-markdown";
import rehypeSanitize from "rehype-sanitize";
import remarkGfm from "remark-gfm";

import { cn } from "@/lib/utils";

type AIMarkdownPreviewProps = {
  className?: string;
  markdown: string;
};

export function AIMarkdownPreview({
  className,
  markdown,
}: AIMarkdownPreviewProps) {
  return (
    <article
      className={cn(
        "prose prose-sm max-w-none dark:prose-invert",
        "prose-pre:overflow-x-auto prose-pre:rounded-md prose-pre:border prose-pre:bg-muted/60",
        "prose-code:rounded prose-code:bg-muted prose-code:px-1 prose-code:py-0.5 prose-code:font-mono prose-code:text-[0.85em]",
        className,
      )}
    >
      <ReactMarkdown
        rehypePlugins={[rehypeSanitize]}
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ children, href }) => (
            <a href={href} rel="noopener noreferrer" target="_blank">
              {children}
            </a>
          ),
        }}
      >
        {markdown}
      </ReactMarkdown>
    </article>
  );
}
