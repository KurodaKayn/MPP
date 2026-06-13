import { Loader2, Trash2 } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";

type ProjectDeleteButtonProps = {
  confirmCancelLabel?: string;
  confirmDescription?: string;
  confirmSubmitLabel?: string;
  confirmTitle?: string;
  disabled?: boolean;
  isDeleting?: boolean;
  label: string;
  onDelete: () => void;
  title?: string;
};

export function ProjectDeleteButton({
  label,
  confirmCancelLabel = "Cancel",
  confirmDescription = label,
  confirmSubmitLabel = label,
  confirmTitle = label,
  disabled,
  isDeleting,
  onDelete,
  title,
}: ProjectDeleteButtonProps) {
  return (
    <AlertDialog>
      <AlertDialogTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={disabled || isDeleting}
            title={title}
          >
            {isDeleting ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Trash2 className="size-4 text-destructive" />
            )}
            <span className="sr-only">{label}</span>
          </Button>
        }
      />
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{confirmTitle}</AlertDialogTitle>
          <AlertDialogDescription>{confirmDescription}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{confirmCancelLabel}</AlertDialogCancel>
          <AlertDialogAction onClick={onDelete}>
            {confirmSubmitLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
