"use client"

import { useState } from "react"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Send, Eye, Save } from "lucide-react"
import { toast } from "sonner"

export default function ContentPage() {
  const [content, setContent] = useState("")
  const [title, setTitle] = useState("")

  const handlePublish = () => {
    if (!title || !content) {
      toast.error("内容不完整", {
        description: "请填写标题和正文后再发布。",
      })
      return
    }
    toast.success("发布中...", {
      description: "内容正在同步到后端服务。",
    })
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-3xl font-bold tracking-tight">内容创作</h2>
          <p className="text-muted-foreground">在此编写您的内容，我们将为您自动适配各平台格式。</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline">
            <Save className="mr-2 h-4 w-4" /> 保存草稿
          </Button>
          <Button onClick={handlePublish}>
            <Send className="mr-2 h-4 w-4" /> 一键发布
          </Button>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        {/* 编辑区 */}
        <Card className="flex flex-col">
          <CardHeader>
            <CardTitle>编辑器</CardTitle>
            <CardDescription>编写您的通用内容</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 flex-1">
            <div className="space-y-2">
              <Label htmlFor="title">标题</Label>
              <Input 
                id="title" 
                placeholder="输入文章标题..." 
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>
            <div className="space-y-2 flex-1 flex flex-col">
              <Label htmlFor="content">正文</Label>
              <Textarea 
                id="content" 
                placeholder="在这里开始创作..." 
                className="min-h-[400px] flex-1 resize-none"
                value={content}
                onChange={(e) => setContent(e.target.value)}
              />
            </div>
          </CardContent>
        </Card>

        {/* 预览区 */}
        <Card className="flex flex-col">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>预览</CardTitle>
                <CardDescription>预览在不同平台的适配效果</CardDescription>
              </div>
              <Eye className="h-4 w-4 text-muted-foreground" />
            </div>
          </CardHeader>
          <CardContent className="flex-1">
            <Tabs defaultValue="wechat" className="w-full">
              <TabsList className="grid w-full grid-cols-4">
                <TabsTrigger value="wechat">公众号</TabsTrigger>
                <TabsTrigger value="zhihu">知乎</TabsTrigger>
                <TabsTrigger value="bilibili">B站</TabsTrigger>
                <TabsTrigger value="xiaohongshu">小红书</TabsTrigger>
              </TabsList>
              <ScrollArea className="h-[500px] w-full rounded-md border p-4 mt-4">
                {!title && !content ? (
                  <div className="h-full flex items-center justify-center text-muted-foreground italic min-h-[400px]">
                    输入内容以预览效果
                  </div>
                ) : (
                  <>
                    <TabsContent value="wechat" className="mt-0">
                      <div className="prose prose-sm dark:prose-invert">
                        <h1 className="text-xl font-bold">{title}</h1>
                        <div className="mt-4 whitespace-pre-wrap">{content}</div>
                      </div>
                    </TabsContent>
                    <TabsContent value="zhihu" className="mt-0">
                      <div className="prose prose-sm dark:prose-invert">
                        <h2 className="text-lg font-bold">{title}</h2>
                        <div className="mt-4 whitespace-pre-wrap">{content}</div>
                      </div>
                    </TabsContent>
                    <TabsContent value="bilibili" className="mt-0">
                      <div className="space-y-4">
                        <div className="font-bold">动态预览：</div>
                        <div className="p-3 bg-muted rounded-lg whitespace-pre-wrap">
                          {title ? `#${title}#\n` : ""}{content}
                        </div>
                      </div>
                    </TabsContent>
                    <TabsContent value="xiaohongshu" className="mt-0">
                      <div className="space-y-4">
                        <div className="aspect-square bg-muted rounded-lg flex items-center justify-center">
                          <span className="text-muted-foreground">首图预览</span>
                        </div>
                        <div className="font-bold">{title}</div>
                        <div className="whitespace-pre-wrap">
                          {content}
                          <div className="text-blue-500 mt-2">#内容发布 #效率工具</div>
                        </div>
                      </div>
                    </TabsContent>
                  </>
                )}
              </ScrollArea>
            </Tabs>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
