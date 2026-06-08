import { ContentPageContent } from "./_components/content-page-content";

type ContentRouteProps = {
  searchParams?: Promise<{
    projectId?: string | string[];
  }>;
};

function firstParam(value: string | string[] | undefined) {
  return Array.isArray(value) ? value[0] : value;
}

export default async function Page({ searchParams }: ContentRouteProps) {
  const params = await searchParams;
  const projectId = firstParam(params?.projectId);

  return <ContentPageContent projectId={projectId} />;
}
