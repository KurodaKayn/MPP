import { fetchDashboard } from "./client";
import type {
  CollabDocument,
  CollabDocumentSession,
  CreateCollabDocumentInput,
  PaginatedCollabDocuments,
  UpdateCollabDocumentInput,
} from "./types";

export type ListCollabDocumentsOptions = {
  limit?: number;
  page?: number;
};

export function listCollabDocuments(options: ListCollabDocumentsOptions = {}) {
  const params = new URLSearchParams({
    page: String(options.page ?? 1),
    limit: String(options.limit ?? 20),
  });

  return fetchDashboard<PaginatedCollabDocuments>(
    `/api/collab/documents?${params.toString()}`,
  );
}

export function createCollabDocument(input: CreateCollabDocumentInput) {
  return fetchDashboard<CollabDocument>("/api/collab/documents", {
    body: JSON.stringify(input),
    method: "POST",
  });
}

export function getCollabDocument(documentId: string) {
  return fetchDashboard<CollabDocument>(
    `/api/collab/documents/${encodeURIComponent(documentId)}`,
  );
}

export function updateCollabDocument(
  documentId: string,
  input: UpdateCollabDocumentInput,
) {
  return fetchDashboard<CollabDocument>(
    `/api/collab/documents/${encodeURIComponent(documentId)}`,
    {
      body: JSON.stringify(input),
      method: "PATCH",
    },
  );
}

export function createCollabDocumentSession(documentId: string) {
  return fetchDashboard<CollabDocumentSession>(
    `/api/collab/documents/${encodeURIComponent(documentId)}/session`,
    {
      method: "POST",
    },
  );
}
