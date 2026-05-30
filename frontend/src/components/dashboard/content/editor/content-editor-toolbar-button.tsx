import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

type ToolbarButtonProps = {
  active?: boolean;
  children: ReactNode;
  disabled?: boolean;
  label: string;
  onClick: () => void;
};

export function ToolbarButton({
  active = false,
  children,
  disabled = false,
  label,
  onClick,
}: ToolbarButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            aria-pressed={active}
            disabled={disabled}
            className={cn(
              "text-muted-foreground hover:text-foreground",
              active && "bg-muted text-foreground ring-1 ring-border",
            )}
            onMouseDown={(event) => event.preventDefault()}
            onClick={onClick}
          >
            {children}
          </Button>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export function ToolbarSeparator() {
  return (
    <Separator
      orientation="vertical"
      className="mx-1 hidden h-6 bg-border sm:block"
    />
  );
}
