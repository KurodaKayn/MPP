import { ContentWorkspace } from "../_components/content-workspace";

type EditContentPageProps = {
  params: Promise<{
    projectId: string;
  }>;
};

export default async function EditContentPage({
  params,
}: EditContentPageProps) {
  const { projectId } = await params;

  return <ContentWorkspace projectId={projectId} />;
}
