import { EditContentPageContent } from "../_components/edit-content-page-content";

export const dynamic = "force-dynamic";
export const dynamicParams = true;

type EditContentRouteProps = {
  params: Promise<{
    locale: string;
    projectId: string;
  }>;
};

export default async function Page({ params }: EditContentRouteProps) {
  const { projectId } = await params;

  return <EditContentPageContent projectId={projectId} />;
}
