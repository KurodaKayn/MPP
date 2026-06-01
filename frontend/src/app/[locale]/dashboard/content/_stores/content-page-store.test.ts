// @vitest-environment jsdom

import { beforeEach, describe, expect, it } from "vitest";
import { emptyContentValue } from "@/lib/content/types";
import { useContentPageStore } from "./content-page-store";

describe("useContentPageStore", () => {
  beforeEach(() => {
    useContentPageStore.getState().resetForCreate();
  });

  it("resets prepublish workspace state back to the create defaults", () => {
    useContentPageStore.setState({
      content: {
        firstImageSrc: "https://example.com/cover.png",
        html: "<p>Body</p>",
        text: "Body",
      },
      contentView: "preview",
      isLoading: true,
      isOpeningXPostIntent: true,
      isPublishing: true,
      isSaving: true,
      isSyncingPrepublish: true,
      loadedProjectId: "project-1",
      prepublishDrafts: {
        wechat: {
          format: "html",
          raw: "<p>Body</p>",
          syncedAt: "2026-05-30T12:00:00.000Z",
        },
      },
      selectedPlatforms: ["wechat", "zhihu"],
      title: "Draft title",
    });

    useContentPageStore.getState().resetForCreate();

    expect(useContentPageStore.getState()).toMatchObject({
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
    });
  });
});
