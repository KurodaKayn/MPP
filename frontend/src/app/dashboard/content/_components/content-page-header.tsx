import { Button } from "@/components/ui/button";
import { Save, Send } from "lucide-react";

type ContentPageHeaderProps = {
  onOpenPublishPanel: () => void;
};

export function ContentPageHeader({
  onOpenPublishPanel,
}: ContentPageHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <h2 className="text-3xl font-bold tracking-tight">内容创作</h2>
        <p className="text-muted-foreground">
          在此编写您的内容，我们将为您自动适配各平台格式。
        </p>
      </div>
      <div className="flex gap-2">
        <Button variant="outline">
          <Save className="mr-2 h-4 w-4" /> 保存草稿
        </Button>
        <Button onClick={onOpenPublishPanel}>
          <Send className="mr-2 h-4 w-4" /> 发布设置
        </Button>
      </div>
    </div>
  );
}
