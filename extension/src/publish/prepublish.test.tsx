import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { BackendApiError } from "../backend/client";
import type { ExtensionPrepublishResponse } from "../backend/types";
import {
  PrepublishWorkbenchCard,
  getPrepublishViewState,
  usePrepublishWorkbench,
} from "./prepublish";
import type { PlatformUiKey } from "./platform-ui";

function createPrepublishResponse(): ExtensionPrepublishResponse {
  return {
    items: [
      {
        project_id: "project-1",
        title: "Douyin article draft",
        status: "ready",
        updated_at: "2026-06-03T10:00:00Z",
        platforms: [
          {
            publication_id: "publication-1",
            platform: "douyin",
            adapter_key: "DYNAMIC_DOUYIN",
            content_kind: "article",
            status: "adapted",
            enabled: true,
            preview: "First draft preview",
          },
          {
            publication_id: "publication-2",
            platform: "bilibili",
            adapter_key: "DYNAMIC_BILIBILI",
            content_kind: "dynamic_post",
            status: "disabled",
            enabled: false,
            preview: "Disabled draft preview",
          },
        ],
      },
      {
        project_id: "project-2",
        title: "Second draft",
        status: "ready",
        updated_at: "2026-06-03T11:00:00Z",
        platforms: [
          {
            publication_id: "publication-3",
            platform: "douyin",
            adapter_key: "DYNAMIC_DOUYIN",
            content_kind: "article",
            status: "adapted",
            enabled: true,
            preview: "Second preview",
          },
        ],
      },
    ],
  };
}

function createXPrepublishResponse(): ExtensionPrepublishResponse {
  return {
    items: [
      {
        project_id: "project-x",
        title: "X draft",
        status: "ready",
        updated_at: "2026-06-03T12:00:00Z",
        platforms: [
          {
            publication_id: "publication-x",
            platform: "x",
            adapter_key: "POST_X",
            content_kind: "dynamic_post",
            status: "adapted",
            enabled: true,
            preview: "X draft preview",
          },
        ],
      },
    ],
  };
}

function createDouyinAndXPrepublishResponse(): ExtensionPrepublishResponse {
  return {
    items: [
      {
        project_id: "project-multi",
        title: "Multi platform draft",
        status: "ready",
        updated_at: "2026-06-03T12:30:00Z",
        platforms: [
          {
            publication_id: "publication-douyin",
            platform: "douyin",
            adapter_key: "DYNAMIC_DOUYIN",
            content_kind: "article",
            status: "adapted",
            enabled: true,
            preview: "Douyin draft preview",
          },
          {
            publication_id: "publication-x",
            platform: "x",
            adapter_key: "POST_X",
            content_kind: "dynamic_post",
            status: "adapted",
            enabled: true,
            preview: "X draft preview",
          },
        ],
      },
    ],
  };
}

describe("getPrepublishViewState", () => {
  it("maps backend prepublish items to loaded state", async () => {
    await expect(
      getPrepublishViewState(() => Promise.resolve(createPrepublishResponse())),
    ).resolves.toMatchObject({
      status: "loaded",
      items: expect.arrayContaining([
        expect.objectContaining({
          project_id: "project-1",
          title: "Douyin article draft",
        }),
      ]),
    });
  });

  it("maps empty backend lists to empty state", async () => {
    await expect(
      getPrepublishViewState(() => Promise.resolve({ items: [] })),
    ).resolves.toEqual({
      status: "empty",
    });
  });

  it("maps backend failures to error state", async () => {
    await expect(
      getPrepublishViewState(() =>
        Promise.reject(
          new BackendApiError("prepublish unavailable", {
            code: "internal_error",
            status: 500,
          }),
        ),
      ),
    ).resolves.toMatchObject({
      status: "error",
      message: "prepublish unavailable",
    });
  });
});

describe("PrepublishWorkbenchCard", () => {
  it("shows projects and enabled platform selection", () => {
    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-1"
        selectedPlatforms={new Set<PlatformUiKey>(["douyin"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={vi.fn()}
        startingHandoff={false}
      />,
    );

    expect(screen.getByText("Pre-Publish Drafts")).toBeInTheDocument();
    expect(
      screen.queryByText("Choose a draft and platform to prepare."),
    ).not.toBeInTheDocument();
    expect(screen.getAllByText("Douyin article draft").length).toBeGreaterThan(
      0,
    );
    expect(
      screen.getByRole("button", { name: /douyin article draft/i }),
    ).toHaveAttribute("data-state", "open");
    expect(screen.getAllByText("ready")).toHaveLength(1);
    expect(
      screen.queryByText("1 ready platform selected"),
    ).not.toBeInTheDocument();
    expect(screen.getByText("First draft preview")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /douyin ready/i }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      within(screen.getByRole("button", { name: /douyin ready/i })).queryByText(
        "First draft preview",
      ),
    ).not.toBeInTheDocument();
    expect(screen.getByAltText("Douyin icon")).toHaveAttribute(
      "src",
      "/icon/platforms/douyin.svg",
    );
    expect(
      screen.getByRole("button", { name: /wechat coming soon/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /x unavailable/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /zhihu coming soon/i }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("switches selected project", () => {
    const selectProject = vi.fn();

    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-1"
        selectedPlatforms={new Set<PlatformUiKey>(["douyin"])}
        onProjectSelect={selectProject}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /second draft/i }));

    expect(selectProject).toHaveBeenCalledWith("project-2");
  });

  it("shows preview for the selected project accordion", () => {
    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-2"
        selectedPlatforms={new Set<PlatformUiKey>(["douyin"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(
      screen.getByRole("button", { name: /second draft/i }),
    ).toHaveAttribute("data-state", "open");
    expect(screen.getByText("Second preview")).toBeInTheDocument();
    expect(screen.queryByText("First draft preview")).not.toBeInTheDocument();
  });

  it("starts handoff for the selected project and platforms", () => {
    const startHandoff = vi.fn();

    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-1"
        selectedPlatforms={new Set<PlatformUiKey>(["douyin"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={startHandoff}
        startingHandoff={false}
      />,
    );

    expect(
      screen.queryByRole("button", { name: /start handoff/i }),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /start publishing/i }));

    expect(startHandoff).toHaveBeenCalledWith("project-1", ["douyin"]);
    expect(
      screen.queryByText("Only Douyin will start now."),
    ).not.toBeInTheDocument();
  });

  it("starts handoff for X when the selected project has an enabled X draft", () => {
    const startHandoff = vi.fn();

    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createXPrepublishResponse().items,
        }}
        selectedProjectId="project-x"
        selectedPlatforms={new Set<PlatformUiKey>(["x"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={startHandoff}
        startingHandoff={false}
      />,
    );

    expect(screen.getByRole("button", { name: /x ready/i })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    fireEvent.click(screen.getByRole("button", { name: /start publishing/i }));

    expect(startHandoff).toHaveBeenCalledWith("project-x", ["x"]);
  });

  it("starts handoff for Douyin and X together when both are selected", () => {
    const startHandoff = vi.fn();

    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createDouyinAndXPrepublishResponse().items,
        }}
        selectedProjectId="project-multi"
        selectedPlatforms={new Set<PlatformUiKey>(["douyin", "x"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={startHandoff}
        startingHandoff={false}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /start publishing/i }));

    expect(startHandoff).toHaveBeenCalledWith("project-multi", ["douyin", "x"]);
    expect(
      screen.queryByText(/Only .* will start now/i),
    ).not.toBeInTheDocument();
  });

  it("keeps UI-only platform selections out of handoff", () => {
    const startHandoff = vi.fn();

    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-1"
        selectedPlatforms={new Set<PlatformUiKey>(["douyin", "wechat"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={startHandoff}
        startingHandoff={false}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /start publishing/i }));

    expect(startHandoff).toHaveBeenCalledWith("project-1", ["douyin"]);
    expect(screen.getByText("Only Douyin will start now.")).toBeInTheDocument();
  });

  it("keeps start disabled until at least one platform is selected", () => {
    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-1"
        selectedPlatforms={new Set<PlatformUiKey>()}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={vi.fn()}
        startingHandoff={false}
      />,
    );

    expect(
      screen.getByRole("button", { name: /start publishing/i }),
    ).toBeDisabled();
    expect(
      screen.queryByText("0 ready platforms selected"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("Select Douyin or X to start publishing."),
    ).toBeInTheDocument();
  });

  it("keeps start disabled when only UI-only platforms are selected", () => {
    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-1"
        selectedPlatforms={new Set<PlatformUiKey>(["wechat"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={vi.fn()}
        startingHandoff={false}
      />,
    );

    expect(
      screen.getByRole("button", { name: /start publishing/i }),
    ).toBeDisabled();
    expect(
      screen.getByText("Select Douyin or X to start publishing."),
    ).toBeInTheDocument();
  });

  it("hides adapter keys and backend statuses from the platform selector", () => {
    render(
      <PrepublishWorkbenchCard
        state={{
          status: "loaded",
          items: createPrepublishResponse().items,
        }}
        selectedProjectId="project-1"
        selectedPlatforms={new Set<PlatformUiKey>(["douyin"])}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
        onStartHandoff={vi.fn()}
        startingHandoff={false}
      />,
    );

    expect(screen.getByText("Douyin")).toBeInTheDocument();
    expect(screen.queryByText("DYNAMIC_DOUYIN")).not.toBeInTheDocument();
    expect(screen.queryByText("adapted")).not.toBeInTheDocument();
  });

  it("uses short task-oriented empty state copy", () => {
    render(
      <PrepublishWorkbenchCard
        state={{ status: "empty" }}
        selectedProjectId={null}
        selectedPlatforms={new Set<PlatformUiKey>()}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByText("No pre-publish drafts yet.")).toBeInTheDocument();
  });

  it("shows retry action for backend errors", () => {
    const retry = vi.fn();

    render(
      <PrepublishWorkbenchCard
        state={{
          status: "error",
          message: "prepublish unavailable",
        }}
        selectedProjectId={null}
        selectedPlatforms={new Set<PlatformUiKey>()}
        onProjectSelect={vi.fn()}
        onPlatformToggle={vi.fn()}
        onRetry={retry}
        onStartHandoff={vi.fn()}
        startingHandoff={false}
      />,
    );

    expect(screen.getByText("prepublish unavailable")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /retry/i }));

    expect(retry).toHaveBeenCalledOnce();
  });
});

describe("usePrepublishWorkbench", () => {
  it("loads prepublish items when enabled", async () => {
    function Harness({
      loadPrepublish,
    }: {
      loadPrepublish: () => Promise<ExtensionPrepublishResponse>;
    }) {
      const workbench = usePrepublishWorkbench(loadPrepublish, true);
      return <PrepublishWorkbenchCard {...workbench} />;
    }
    const loadPrepublish = vi
      .fn()
      .mockResolvedValue(createPrepublishResponse());

    render(<Harness loadPrepublish={loadPrepublish} />);

    expect(screen.getByText("loading")).toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getAllByText("Douyin article draft").length,
      ).toBeGreaterThan(0),
    );
    expect(loadPrepublish).toHaveBeenCalledOnce();
    expect(
      screen.getByRole("button", { name: /douyin ready/i }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("waits for an authenticated session before loading", () => {
    function Harness({
      loadPrepublish,
    }: {
      loadPrepublish: () => Promise<ExtensionPrepublishResponse>;
    }) {
      const workbench = usePrepublishWorkbench(loadPrepublish, false);
      return <PrepublishWorkbenchCard {...workbench} />;
    }
    const loadPrepublish = vi.fn();

    render(<Harness loadPrepublish={loadPrepublish} />);

    expect(
      screen.getByText("Sign in to MPP to load drafts."),
    ).toBeInTheDocument();
    expect(screen.queryByText("idle")).not.toBeInTheDocument();
    expect(loadPrepublish).not.toHaveBeenCalled();
  });

  it("shows compact login actions while waiting for authentication", () => {
    function Harness({
      loadPrepublish,
      openLogin,
    }: {
      loadPrepublish: () => Promise<ExtensionPrepublishResponse>;
      openLogin: () => void;
    }) {
      const workbench = usePrepublishWorkbench(loadPrepublish, false);
      return <PrepublishWorkbenchCard {...workbench} onOpenLogin={openLogin} />;
    }
    const loadPrepublish = vi.fn();
    const openLogin = vi.fn();

    render(<Harness loadPrepublish={loadPrepublish} openLogin={openLogin} />);

    expect(
      screen.getByText("Sign in to MPP to load drafts."),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /open mpp/i }));

    expect(openLogin).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument();
    expect(loadPrepublish).not.toHaveBeenCalled();
  });
});
