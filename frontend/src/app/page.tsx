import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import {
  ArrowRight,
  Bot,
  CheckCircle2,
  FileText,
  Send,
  ShieldCheck,
  Sparkles,
  Workflow,
  type LucideIcon,
} from "lucide-react";
import { PLATFORM_TABS } from "@/lib/content/platforms";
import { siteConfig } from "@/lib/seo";

export const metadata: Metadata = {
  title: "多平台内容发布工作台",
  description: siteConfig.description,
  alternates: {
    canonical: "/",
  },
};

const workflowSteps = [
  {
    title: "写一份源内容",
    description: "把项目标题、正文和素材集中在同一个工作台里管理。",
    icon: FileText,
  },
  {
    title: "生成平台草稿",
    description: "为公众号、知乎、X、B站、小红书维护独立的适配稿。",
    icon: Workflow,
  },
  {
    title: "用 AI 审阅改写",
    description: "AI 输出先进入提案区，确认后才会更新当前内容。",
    icon: Bot,
  },
  {
    title: "追踪发布状态",
    description: "同步查看草稿、就绪、发布中、已发布和失败状态。",
    icon: Send,
  },
] satisfies Array<{
  title: string;
  description: string;
  icon: LucideIcon;
}>;

const featureGroups = [
  {
    title: "统一项目空间",
    body: "源内容、平台草稿、发布记录和账号连接收在一个清晰的工作流里，减少多窗口切换。",
  },
  {
    title: "平台差异可见",
    body: "每个平台保留自己的草稿和状态，适合处理标题、格式、长度和发布方式不同的渠道。",
  },
  {
    title: "AI 改写可控",
    body: "AI 只生成可审阅提案，团队可以比较差异后再接受，避免直接覆盖原稿。",
  },
] as const;

const useCases = [
  "内容运营团队管理多渠道发布节奏",
  "创作者把长文拆成不同平台版本",
  "品牌团队维护发布状态和失败重试",
  "技术团队扩展新的平台适配器",
] as const;

const jsonLd = JSON.stringify([
  {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: siteConfig.name,
    alternateName: siteConfig.shortName,
    url: siteConfig.url,
    inLanguage: "zh-CN",
  },
  {
    "@context": "https://schema.org",
    "@type": "SoftwareApplication",
    name: siteConfig.name,
    alternateName: siteConfig.shortName,
    applicationCategory: "BusinessApplication",
    operatingSystem: "Web",
    url: siteConfig.url,
    description: siteConfig.description,
    inLanguage: "zh-CN",
    featureList: [
      "多平台内容项目管理",
      "平台草稿适配",
      "AI 辅助编辑提案",
      "发布状态追踪",
      "远程浏览器账号连接",
    ],
  },
]).replace(/</g, "\\u003c");

function HeroScene() {
  return (
    <div
      className="pointer-events-none absolute inset-0 overflow-hidden"
      aria-hidden="true"
    >
      <div className="absolute inset-0 bg-[#f5f7f1]" />
      <div className="absolute inset-0 opacity-[0.13] [background-image:linear-gradient(#17211c_1px,transparent_1px),linear-gradient(90deg,#17211c_1px,transparent_1px)] [background-size:52px_52px]" />

      <div className="absolute right-[-420px] top-28 hidden w-[760px] xl:block 2xl:right-[-280px]">
        <div className="grid rotate-[-4deg] grid-cols-[220px_1fr] gap-3">
          <div className="space-y-3">
            <div className="rounded-[8px] border border-[#17211c]/15 bg-white/85 p-4 shadow-[0_18px_60px_rgba(23,33,28,0.12)]">
              <div className="mb-4 h-2 w-16 rounded-full bg-[#e5533d]" />
              <div className="space-y-2">
                <div className="h-3 rounded-full bg-[#17211c]/80" />
                <div className="h-3 w-4/5 rounded-full bg-[#17211c]/35" />
                <div className="h-3 w-2/3 rounded-full bg-[#17211c]/25" />
              </div>
            </div>
            <div className="rounded-[8px] border border-[#17211c]/15 bg-[#0f6f78] p-4 text-white shadow-[0_18px_60px_rgba(15,111,120,0.18)]">
              <div className="text-xs uppercase tracking-normal text-white/70">
                Drafts
              </div>
              <div className="mt-4 grid grid-cols-2 gap-2">
                {PLATFORM_TABS.slice(0, 4).map((platform) => (
                  <div
                    key={platform.value}
                    className="flex h-12 items-center justify-center rounded-[8px] bg-white/12"
                  >
                    <Image src={platform.icon} alt="" width={22} height={22} />
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div className="rounded-[8px] border border-[#17211c]/15 bg-white/90 p-4 shadow-[0_24px_80px_rgba(23,33,28,0.14)]">
            <div className="mb-4 flex items-center justify-between border-b border-[#17211c]/10 pb-3">
              <div className="flex items-center gap-2">
                <span className="h-2.5 w-2.5 rounded-full bg-[#e5533d]" />
                <span className="h-2.5 w-2.5 rounded-full bg-[#f5b544]" />
                <span className="h-2.5 w-2.5 rounded-full bg-[#0f8b5f]" />
              </div>
              <div className="h-2 w-20 rounded-full bg-[#17211c]/20" />
            </div>
            <div className="grid grid-cols-[1fr_150px] gap-4">
              <div className="space-y-3">
                <div className="h-5 w-3/4 rounded-full bg-[#17211c]" />
                <div className="h-3 rounded-full bg-[#17211c]/28" />
                <div className="h-3 w-11/12 rounded-full bg-[#17211c]/22" />
                <div className="h-3 w-4/5 rounded-full bg-[#17211c]/18" />
                <div className="mt-5 grid grid-cols-3 gap-2">
                  {["源稿", "适配", "发布"].map((item, index) => (
                    <div
                      key={item}
                      className="rounded-[8px] border border-[#17211c]/10 bg-[#f5f7f1] p-3"
                    >
                      <div
                        className={`mb-3 h-2 w-10 rounded-full ${
                          index === 0
                            ? "bg-[#e5533d]"
                            : index === 1
                              ? "bg-[#0f6f78]"
                              : "bg-[#0f8b5f]"
                        }`}
                      />
                      <div className="text-xs font-medium text-[#17211c]">
                        {item}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
              <div className="space-y-2">
                {PLATFORM_TABS.map((platform) => (
                  <div
                    key={platform.value}
                    className="flex items-center gap-2 rounded-[8px] border border-[#17211c]/10 bg-white p-2"
                  >
                    <Image src={platform.icon} alt="" width={18} height={18} />
                    <div className="h-2 flex-1 rounded-full bg-[#17211c]/20" />
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function StepCard({
  description,
  icon: Icon,
  index,
  title,
}: {
  description: string;
  icon: LucideIcon;
  index: number;
  title: string;
}) {
  return (
    <article className="rounded-[8px] border border-[#17211c]/12 bg-white p-5 shadow-[0_14px_45px_rgba(23,33,28,0.06)]">
      <div className="mb-7 flex items-center justify-between">
        <div className="flex h-10 w-10 items-center justify-center rounded-[8px] bg-[#17211c] text-white">
          <Icon className="h-5 w-5" />
        </div>
        <span className="font-mono text-sm text-[#e5533d]">0{index + 1}</span>
      </div>
      <h3 className="text-lg font-semibold tracking-normal text-[#17211c]">
        {title}
      </h3>
      <p className="mt-3 text-sm leading-6 text-[#52615a]">{description}</p>
    </article>
  );
}

export default function Home() {
  return (
    <main className="bg-[#f5f7f1] text-[#17211c]">
      <script
        type="application/ld+json"
        suppressHydrationWarning
        dangerouslySetInnerHTML={{ __html: jsonLd }}
      />

      <section className="relative min-h-[88svh] overflow-hidden border-b border-[#17211c]/12">
        <HeroScene />
        <div className="relative z-10 mx-auto flex min-h-[88svh] w-full max-w-7xl flex-col px-5 py-5 sm:px-8 lg:px-10">
          <header className="flex items-center justify-between gap-4">
            <Link href="/" className="flex items-center gap-3">
              <Image
                src="/icons/mpp.svg"
                alt="multi-platform poster"
                width={36}
                height={36}
                priority
              />
              <span className="hidden text-sm font-semibold tracking-normal sm:inline">
                multi-platform poster
              </span>
            </Link>
            <nav className="hidden items-center gap-7 text-sm text-[#52615a] md:flex">
              <Link href="#workflow" className="hover:text-[#17211c]">
                工作流
              </Link>
              <Link href="#platforms" className="hover:text-[#17211c]">
                平台
              </Link>
              <Link href="#use-cases" className="hover:text-[#17211c]">
                场景
              </Link>
            </nav>
            <Link
              href="/login"
              className="inline-flex h-10 items-center gap-2 rounded-[8px] border border-[#17211c] bg-[#17211c] px-4 text-sm font-medium text-white transition hover:bg-[#0f6f78]"
            >
              进入工作台
              <ArrowRight className="h-4 w-4" />
            </Link>
          </header>

          <div className="flex flex-1 items-center py-16 md:py-20">
            <div className="max-w-3xl">
              <div className="mb-5 inline-flex items-center gap-2 rounded-[8px] border border-[#0f6f78]/30 bg-white/70 px-3 py-1 text-sm text-[#0f6f78]">
                <Sparkles className="h-4 w-4" />
                内容项目、平台草稿、发布状态统一管理
              </div>
              <h1 className="text-5xl font-semibold leading-[1.02] tracking-normal text-[#17211c] md:text-7xl">
                multi-platform poster
              </h1>
              <p className="mt-6 max-w-2xl text-lg leading-8 text-[#3f4d46]">
                从一篇原始内容出发，统一管理公众号、知乎、X、B站和小红书的草稿适配、发布准备、状态追踪与
                AI 编辑。
              </p>

              <div className="mt-8 flex flex-col gap-3 sm:flex-row">
                <Link
                  href="/login"
                  className="inline-flex h-12 items-center justify-center gap-2 rounded-[8px] bg-[#e5533d] px-5 text-sm font-semibold text-white transition hover:bg-[#c64230]"
                >
                  开始发布管理
                  <ArrowRight className="h-4 w-4" />
                </Link>
                <Link
                  href="#workflow"
                  className="inline-flex h-12 items-center justify-center gap-2 rounded-[8px] border border-[#17211c]/20 bg-white/75 px-5 text-sm font-semibold text-[#17211c] transition hover:border-[#17211c]/45"
                >
                  查看工作流
                </Link>
              </div>

              <div className="mt-10 flex flex-wrap items-center gap-3">
                {PLATFORM_TABS.map((platform) => (
                  <span
                    key={platform.value}
                    className="inline-flex items-center gap-2 rounded-[8px] border border-[#17211c]/12 bg-white/70 px-3 py-2 text-sm text-[#3f4d46]"
                  >
                    <Image
                      src={platform.icon}
                      alt={platform.label}
                      width={18}
                      height={18}
                    />
                    {platform.label}
                  </span>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section
        id="workflow"
        className="border-b border-[#17211c]/12 bg-[#fbfbf7] py-20"
      >
        <div className="mx-auto max-w-7xl px-5 sm:px-8 lg:px-10">
          <div className="max-w-2xl">
            <p className="text-sm font-semibold uppercase tracking-normal text-[#0f6f78]">
              Publishing workflow
            </p>
            <h2 className="mt-3 text-3xl font-semibold tracking-normal md:text-5xl">
              把内容发布拆成可管理的四步。
            </h2>
          </div>
          <div className="mt-10 grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            {workflowSteps.map((step, index) => (
              <StepCard
                key={step.title}
                description={step.description}
                icon={step.icon}
                index={index}
                title={step.title}
              />
            ))}
          </div>
        </div>
      </section>

      <section id="platforms" className="bg-white py-20">
        <div className="mx-auto grid max-w-7xl gap-10 px-5 sm:px-8 lg:grid-cols-[0.85fr_1.15fr] lg:px-10">
          <div>
            <p className="text-sm font-semibold uppercase tracking-normal text-[#e5533d]">
              Platform-ready drafts
            </p>
            <h2 className="mt-3 text-3xl font-semibold tracking-normal md:text-5xl">
              每个平台都有自己的草稿和状态。
            </h2>
            <p className="mt-5 text-base leading-7 text-[#52615a]">
              MPP
              把源内容和平台发布记录分开管理，既保留统一编辑入口，也允许不同平台拥有各自的格式、配置和发布结果。
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            {featureGroups.map((feature) => (
              <article
                key={feature.title}
                className="rounded-[8px] border border-[#17211c]/12 bg-[#f5f7f1] p-5"
              >
                <CheckCircle2 className="h-5 w-5 text-[#0f8b5f]" />
                <h3 className="mt-5 text-lg font-semibold tracking-normal">
                  {feature.title}
                </h3>
                <p className="mt-3 text-sm leading-6 text-[#52615a]">
                  {feature.body}
                </p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section
        id="use-cases"
        className="border-y border-[#17211c]/12 bg-[#17211c] py-20 text-white"
      >
        <div className="mx-auto grid max-w-7xl gap-10 px-5 sm:px-8 lg:grid-cols-[1fr_1fr] lg:px-10">
          <div>
            <div className="mb-6 inline-flex h-12 w-12 items-center justify-center rounded-[8px] bg-[#f5b544] text-[#17211c]">
              <ShieldCheck className="h-6 w-6" />
            </div>
            <h2 className="text-3xl font-semibold tracking-normal md:text-5xl">
              面向需要稳定发布流程的团队。
            </h2>
          </div>
          <div className="grid gap-3">
            {useCases.map((item) => (
              <div
                key={item}
                className="flex items-start gap-3 rounded-[8px] border border-white/12 bg-white/7 p-4"
              >
                <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-[#7bd88f]" />
                <span className="leading-7 text-white/84">{item}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="bg-[#fbfbf7] py-16">
        <div className="mx-auto flex max-w-7xl flex-col gap-6 px-5 sm:px-8 md:flex-row md:items-center md:justify-between lg:px-10">
          <div>
            <h2 className="text-2xl font-semibold tracking-normal">
              从公开首页开始，把搜索流量接到真实产品。
            </h2>
            <p className="mt-2 text-sm leading-6 text-[#52615a]">
              首页已经为搜索引擎提供产品定位、平台覆盖、工作流和结构化数据。
            </p>
          </div>
          <Link
            href="/login"
            className="inline-flex h-11 items-center justify-center gap-2 rounded-[8px] bg-[#17211c] px-5 text-sm font-semibold text-white transition hover:bg-[#0f6f78]"
          >
            进入工作台
            <ArrowRight className="h-4 w-4" />
          </Link>
        </div>
      </section>
    </main>
  );
}
