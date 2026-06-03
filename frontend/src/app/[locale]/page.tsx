import { HomePage } from "./_components/home-page";
export { generateHomeMetadata as generateMetadata } from "./_lib/home-seo";

type HomeRouteProps = {
  params: Promise<{ locale: string }>;
};

export default async function Page({ params }: HomeRouteProps) {
  const { locale } = await params;

  return <HomePage locale={locale} />;
}
