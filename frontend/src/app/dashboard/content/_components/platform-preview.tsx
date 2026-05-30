import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Eye } from "lucide-react";
import Image from "next/image";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import type { ContentValue } from "@/lib/content/types";

type PlatformPreviewProps = {
  title: string;
  content: ContentValue;
  viewSwitcher?: React.ReactNode;
};

function PlatformTabLabel({ icon, label }: { icon: string; label: string }) {
  return (
    <>
      <Image
        src={icon}
        alt=""
        width={16}
        height={16}
        aria-hidden="true"
        className="size-4 shrink-0"
      />
      <span>{label}</span>
    </>
  );
}

export function PlatformPreview({
  title,
  content,
  viewSwitcher,
}: PlatformPreviewProps) {
  const hasBodyContent = Boolean(content.text.trim() || content.firstImageSrc);
  const previewContent = (
    <div
      className="mt-4 space-y-4 whitespace-pre-wrap [&_figcaption]:border-t [&_figcaption]:bg-background/90 [&_figcaption]:px-3 [&_figcaption]:py-2 [&_figcaption]:text-xs [&_figcaption]:text-muted-foreground [&_figure]:overflow-hidden [&_figure]:rounded-lg [&_figure]:border [&_figure]:bg-muted [&_img]:max-h-96 [&_img]:w-full [&_img]:object-contain"
      dangerouslySetInnerHTML={{ __html: content.html }}
    />
  );

  return (
    <Card className="flex flex-col">
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>预览</CardTitle>
            <CardDescription>预览在不同平台的适配效果</CardDescription>
          </div>
          <div className="flex items-center gap-3">
            {viewSwitcher}
            <Eye className="h-4 w-4 text-muted-foreground" />
          </div>
        </div>
      </CardHeader>
      <CardContent className="flex-1">
        <Tabs defaultValue={PLATFORM_TABS[0].value} className="w-full">
          <TabsList
            className="grid w-full"
            style={{
              gridTemplateColumns: `repeat(${PLATFORM_TABS.length}, minmax(0, 1fr))`,
            }}
          >
            {PLATFORM_TABS.map((platform) => (
              <TabsTrigger key={platform.value} value={platform.value}>
                <PlatformTabLabel icon={platform.icon} label={platform.label} />
              </TabsTrigger>
            ))}
          </TabsList>
          <ScrollArea className="h-[500px] w-full rounded-md border p-4 mt-4">
            {!title && !hasBodyContent ? (
              <div className="h-full flex items-center justify-center text-muted-foreground italic min-h-[400px]">
                输入内容以预览效果
              </div>
            ) : (
              <>
                <TabsContent value="wechat" className="mt-0">
                  <div className="prose prose-sm dark:prose-invert">
                    <h1 className="text-xl font-bold">{title}</h1>
                    {previewContent}
                  </div>
                </TabsContent>
                <TabsContent value="zhihu" className="mt-0">
                  <div className="prose prose-sm dark:prose-invert">
                    <h2 className="text-lg font-bold">{title}</h2>
                    {previewContent}
                  </div>
                </TabsContent>
                <TabsContent value="x" className="mt-0">
                  <div className="rounded-lg border bg-background p-4">
                    <div className="flex items-center gap-3">
                      <div className="flex size-10 items-center justify-center rounded-full bg-foreground text-sm font-semibold text-background">
                        X
                      </div>
                      <div className="min-w-0">
                        <div className="font-semibold">MPP</div>
                        <div className="text-sm text-muted-foreground">
                          @mpp
                        </div>
                      </div>
                    </div>
                    <div className="mt-4 whitespace-pre-wrap text-sm leading-6">
                      {title ? `${title}\n\n` : ""}
                      {content.text}
                    </div>
                    <div className="mt-3 text-xs text-muted-foreground">
                      {
                        Array.from(
                          `${title ? `${title}\n\n` : ""}${content.text}`,
                        ).length
                      }
                      /280
                    </div>
                  </div>
                </TabsContent>
                <TabsContent value="bilibili" className="mt-0">
                  <div className="space-y-4">
                    <div className="font-bold">动态预览：</div>
                    <div className="p-3 bg-muted rounded-lg whitespace-pre-wrap">
                      {title ? `#${title}#\n` : ""}
                      {content.text}
                    </div>
                    {content.firstImageSrc ? previewContent : null}
                  </div>
                </TabsContent>
                <TabsContent value="xiaohongshu" className="mt-0">
                  <div className="space-y-4">
                    <div className="aspect-square bg-muted rounded-lg flex items-center justify-center overflow-hidden">
                      {content.firstImageSrc ? (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={content.firstImageSrc}
                          alt="首图预览"
                          className="h-full w-full object-cover"
                        />
                      ) : (
                        <span className="text-muted-foreground">首图预览</span>
                      )}
                    </div>
                    <div className="font-bold">{title}</div>
                    <div className="whitespace-pre-wrap">
                      {content.text}
                      <div className="text-blue-500 mt-2">
                        #内容发布 #效率工具
                      </div>
                    </div>
                  </div>
                </TabsContent>
              </>
            )}
          </ScrollArea>
        </Tabs>
      </CardContent>
    </Card>
  );
}
