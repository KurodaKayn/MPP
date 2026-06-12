import { Loader2, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";

type ProjectDeleteButtonProps = {
  disabled?: boolean;
  isDeleting?: boolean;
  label: string;
  onDelete: () => void;
  title?: string;
};

export function ProjectDeleteButton({
  disabled,
  isDeleting,
  label,
  onDelete,
  title,
}: ProjectDeleteButtonProps) {
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      disabled={disabled || isDeleting}
      title={title}
      onClick={onDelete}
    >
      {isDeleting ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <Trash2 className="size-4 text-destructive" />
      )}
      <span className="sr-only">{label}</span>
    </Button>
  );
}
