"use client";

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { ExternalLink, HelpCircle, X } from "lucide-react";
import Image from "next/image";

import { Button } from "@/components/ui/button";

export function WechatCredentialGuideDialog() {
  return (
    <DialogPrimitive.Root>
      <DialogPrimitive.Trigger
        render={
          <Button type="button" variant="outline" size="sm">
            <HelpCircle className="size-3.5" />
            获取凭证
          </Button>
        }
      />
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/20 transition-opacity duration-150 data-ending-style:opacity-0 data-starting-style:opacity-0 supports-backdrop-filter:backdrop-blur-xs" />
        <DialogPrimitive.Popup className="fixed top-1/2 left-1/2 z-50 grid max-h-[min(760px,calc(100vh-2rem))] w-[calc(100vw-2rem)] max-w-3xl -translate-x-1/2 -translate-y-1/2 grid-rows-[auto_minmax(0,1fr)] overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-xl transition duration-150 data-ending-style:scale-95 data-ending-style:opacity-0 data-starting-style:scale-95 data-starting-style:opacity-0">
          <div className="border-b px-5 py-4 pr-14">
            <DialogPrimitive.Title className="font-heading text-base font-medium">
              如何获取 AppID 和 AppSecret
            </DialogPrimitive.Title>
            <DialogPrimitive.Description className="mt-1 text-sm text-muted-foreground">
              在微信公众平台开发设置中获取凭证，并按测试报错提示配置 IP 白名单。
            </DialogPrimitive.Description>
          </div>
          <div className="overflow-y-auto px-5 py-4">
            <div className="grid gap-5">
              <section className="grid gap-2">
                <div className="text-sm font-medium">1. 打开开发设置</div>
                <a
                  className="inline-flex w-fit items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-sm font-medium text-primary hover:bg-muted"
                  href="https://developers.weixin.qq.com/console/index?tab1=business&tab2=dev"
                  target="_blank"
                  rel="noreferrer"
                >
                  微信公众平台开发设置
                  <ExternalLink className="size-3.5" />
                </a>
              </section>

              <section className="grid gap-3">
                <div>
                  <div className="text-sm font-medium">2. 点击公众号</div>
                  <p className="mt-1 text-sm leading-6 text-muted-foreground">
                    如果还没有公众号，先按微信公众平台流程完成注册。
                  </p>
                </div>
                <Image
                  className="w-full rounded-lg border bg-muted object-contain"
                  src="/tutorial/wechat/open-gongzhonghao-console.jpg"
                  alt="微信公众平台控制台中点击公众号入口"
                  width={931}
                  height={1025}
                />
              </section>

              <section className="grid gap-3">
                <div>
                  <div className="text-sm font-medium">
                    3. 获取密钥并设置 IP 白名单
                  </div>
                  <p className="mt-1 text-sm leading-6 text-muted-foreground">
                    可以先保存密钥后点击测试连接，再用报错信息中的 IP
                    设置白名单。
                  </p>
                </div>
                <Image
                  className="w-full rounded-lg border bg-muted object-contain"
                  src="/tutorial/wechat/set-whitelist.jpg"
                  alt="微信公众平台开发设置中获取密钥并配置 IP 白名单"
                  width={1236}
                  height={603}
                />
              </section>
            </div>
          </div>
          <DialogPrimitive.Close
            render={
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="absolute top-3 right-3"
              >
                <X className="size-4" />
                <span className="sr-only">关闭</span>
              </Button>
            }
          />
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
