import { ContentWorkspace } from "./content-workspace";

type EditContentPageContentProps = {
  projectId: string;
};

export function EditContentPageContent({
  projectId,
}: EditContentPageContentProps) {
  return <ContentWorkspace projectId={projectId} />;
}
