"use client";

import { create } from "zustand";
import { emptyContentValue, type ContentValue } from "@/lib/content/types";
import type { PublishPlatform } from "../_lib/publish-content";

export type ContentView = "editor" | "preview";
export type PrepublishFormat = "html" | "markdown" | "text";

export type PrepublishDraft = {
  format: PrepublishFormat;
  raw: string;
  syncedAt: string;
};

type ContentPageState = {
  content: ContentValue;
  contentView: ContentView;
  isLoading: boolean;
  isOpeningXPostIntent: boolean;
  isPublishing: boolean;
  isSaving: boolean;
  isSyncingPrepublish: boolean;
  loadedProjectId: string | null;
  prepublishDrafts: Partial<Record<PublishPlatform, PrepublishDraft>>;
  selectedPlatforms: PublishPlatform[];
  title: string;
};

type ContentPageActions = {
  resetForCreate: () => void;
  setContent: (content: ContentValue) => void;
  setContentView: (contentView: ContentView) => void;
  setIsLoading: (isLoading: boolean) => void;
  setIsOpeningXPostIntent: (isOpeningXPostIntent: boolean) => void;
  setIsPublishing: (isPublishing: boolean) => void;
  setIsSaving: (isSaving: boolean) => void;
  setIsSyncingPrepublish: (isSyncingPrepublish: boolean) => void;
  setLoadedProjectId: (loadedProjectId: string | null) => void;
  setPrepublishDrafts: (
    prepublishDrafts: Partial<Record<PublishPlatform, PrepublishDraft>>,
  ) => void;
  setSelectedPlatforms: (selectedPlatforms: PublishPlatform[]) => void;
  setTitle: (title: string) => void;
};

type ContentPageStore = ContentPageState & ContentPageActions;

const initialState: ContentPageState = {
  content: emptyContentValue,
  contentView: "editor",
  isLoading: false,
  isOpeningXPostIntent: false,
  isPublishing: false,
  isSaving: false,
  isSyncingPrepublish: false,
  loadedProjectId: null,
  prepublishDrafts: {},
  selectedPlatforms: [],
  title: "",
};

export const useContentPageStore = create<ContentPageStore>((set) => ({
  ...initialState,
  resetForCreate: () => set(initialState),
  setContent: (content) => set({ content }),
  setContentView: (contentView) => set({ contentView }),
  setIsLoading: (isLoading) => set({ isLoading }),
  setIsOpeningXPostIntent: (isOpeningXPostIntent) =>
    set({ isOpeningXPostIntent }),
  setIsPublishing: (isPublishing) => set({ isPublishing }),
  setIsSaving: (isSaving) => set({ isSaving }),
  setIsSyncingPrepublish: (isSyncingPrepublish) => set({ isSyncingPrepublish }),
  setLoadedProjectId: (loadedProjectId) => set({ loadedProjectId }),
  setPrepublishDrafts: (prepublishDrafts) => set({ prepublishDrafts }),
  setSelectedPlatforms: (selectedPlatforms) => set({ selectedPlatforms }),
  setTitle: (title) => set({ title }),
}));
