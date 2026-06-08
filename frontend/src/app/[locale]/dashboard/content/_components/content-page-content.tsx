import { ContentWorkspace } from "./content-workspace";

type ContentPageContentProps = {
  projectId?: string;
};

export function ContentPageContent({ projectId }: ContentPageContentProps) {
  return <ContentWorkspace projectId={projectId} />;
}
