import { ShareProjectPage } from "../_components/share-project-page";

type ShareProjectTokenRouteProps = {
  params: Promise<{
    token?: string;
  }>;
};

export default async function Page({ params }: ShareProjectTokenRouteProps) {
  const { token } = await params;

  return <ShareProjectPage token={token} />;
}
