import { ImageResponse } from "next/og";
import { siteConfig } from "@/lib/seo";
import { useTranslation } from "@/lib/i18n";

export const alt = "multi-platform poster product preview";
export const size = {
  width: 1200,
  height: 630,
};
export const contentType = "image/png";

export default async function Image({
  params,
}: {
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  const { t } = await useTranslation(locale, "home");
  const { t: tCommon } = await useTranslation(locale, "common");

  const platforms = [
    tCommon("platforms.wechat"),
    tCommon("platforms.zhihu"),
    tCommon("platforms.x"),
    tCommon("platforms.bilibili"),
    tCommon("platforms.xiaohongshu"),
  ];

  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        background: "#f5f7f1",
        color: "#17211c",
        fontFamily: "Arial, sans-serif",
        padding: 64,
        position: "relative",
      }}
    >
      <div
        style={{
          position: "absolute",
          inset: 0,
          backgroundImage:
            "linear-gradient(#17211c22 1px, transparent 1px), linear-gradient(90deg, #17211c22 1px, transparent 1px)",
          backgroundSize: "54px 54px",
        }}
      />
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          width: "100%",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 18 }}>
          <div
            style={{
              width: 52,
              height: 52,
              borderRadius: 8,
              background: "#17211c",
              color: "#ffffff",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 24,
              fontWeight: 700,
            }}
          >
            M
          </div>
          <div style={{ fontSize: 30, fontWeight: 700 }}>{siteConfig.name}</div>
        </div>

        <div
          style={{
            maxWidth: 820,
            display: "flex",
            flexDirection: "column",
          }}
        >
          <div
            style={{
              color: "#0f6f78",
              fontSize: 28,
              marginBottom: 20,
              fontWeight: 700,
            }}
          >
            {t("title")}
          </div>
          <div
            style={{
              fontSize: 72,
              lineHeight: 1.06,
              fontWeight: 800,
            }}
          >
            One source. Every platform-ready draft.
          </div>
        </div>

        <div style={{ display: "flex", gap: 14 }}>
          {platforms.map((platform) => (
            <div
              key={platform}
              style={{
                border: "1px solid #17211c24",
                borderRadius: 8,
                background: "#ffffffcc",
                padding: "12px 18px",
                fontSize: 22,
                fontWeight: 700,
              }}
            >
              {platform}
            </div>
          ))}
        </div>
      </div>
    </div>,
    size,
  );
}
