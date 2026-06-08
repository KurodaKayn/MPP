import { ShareProjectPage } from "./_components/share-project-page";

type ShareProjectRouteProps = {
  searchParams?: Promise<{
    token?: string | string[];
  }>;
};

function firstParam(value: string | string[] | undefined) {
  return Array.isArray(value) ? value[0] : value;
}

export default async function Page({ searchParams }: ShareProjectRouteProps) {
  const params = await searchParams;
  const token = firstParam(params?.token);

  return <ShareProjectPage token={token} />;
}
