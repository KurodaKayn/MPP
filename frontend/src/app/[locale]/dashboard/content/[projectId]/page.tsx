import { EditContentPageContent } from "../_components/edit-content-page-content";

type EditContentRouteProps = {
  params: Promise<{
    projectId: string;
  }>;
};

export default async function Page({ params }: EditContentRouteProps) {
  const { projectId } = await params;

  return <EditContentPageContent projectId={projectId} />;
}
