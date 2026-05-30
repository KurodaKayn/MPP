import { Button } from "@/components/ui/button";
import { Loader2, Save, Send } from "lucide-react";

type ContentPageHeaderProps = {
  canSave?: boolean;
  isSaving?: boolean;
  mode?: "create" | "edit";
  onOpenPublishPanel: () => void;
  onSave?: () => void;
};

export function ContentPageHeader({
  canSave = false,
  isSaving = false,
  mode = "create",
  onOpenPublishPanel,
  onSave,
}: ContentPageHeaderProps) {
  const isEditing = mode === "edit";

  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h2 className="text-3xl font-bold tracking-tight">
          {isEditing ? "编辑内容" : "内容创作"}
        </h2>
        <p className="text-muted-foreground">
          {isEditing
            ? "修改当前项目内容，并重新选择需要同步的平台。"
            : "在此编写您的内容，我们将为您自动适配各平台格式。"}
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {onSave ? (
          <Button
            type="button"
            variant="outline"
            onClick={onSave}
            disabled={!canSave || isSaving}
          >
            {isSaving ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <Save className="mr-2 h-4 w-4" />
            )}
            保存修改
          </Button>
        ) : null}
        <Button onClick={onOpenPublishPanel}>
          <Send className="mr-2 h-4 w-4" /> 发布设置
        </Button>
      </div>
    </div>
  );
}
