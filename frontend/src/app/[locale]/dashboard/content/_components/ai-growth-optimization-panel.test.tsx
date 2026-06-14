// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { createAIGrowthOptimizationRun } from "@/lib/dashboard/api";
import { AIGrowthOptimizationPanel } from "./ai-growth-optimization-panel";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "en",
  useTranslation: () => ({
    t: (_key: string, options?: { defaultValue?: string }) =>
      options?.defaultValue ?? _key,
  }),
}));

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

vi.mock("@/lib/dashboard/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/dashboard/api")>();

  return {
    ...actual,
    createAIGrowthOptimizationRun: vi.fn(actual.createAIGrowthOptimizationRun),
  };
});

function render(element: ReactElement) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
  });

  return {
    button(name: string) {
      const button = findButton(container, name);
      if (!button) {
        throw new Error(`Button not found: ${name}`);
      }
      return button as HTMLButtonElement;
    },
    click(target: Element) {
      act(() => {
        target.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      });
    },
    select(labelText: string, value: string) {
      const labels = Array.from(container.querySelectorAll("label"));
      const label = labels.find((item) =>
        item.textContent?.toLowerCase().includes(labelText.toLowerCase()),
      );
      const controlId = label?.getAttribute("for");
      if (!controlId) {
        throw new Error(`Select label not found: ${labelText}`);
      }
      const select = container.querySelector<HTMLSelectElement>(
        `#${controlId}`,
      );
      if (!select) {
        throw new Error(`Select control not found: ${labelText}`);
      }
      act(() => {
        select.value = value;
        select.dispatchEvent(new Event("change", { bubbles: true }));
      });
    },
    text() {
      return container.textContent ?? "";
    },
    maybeButton(name: string) {
      return findButton(container, name);
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

function findButton(container: Element, name: string) {
  const buttons = Array.from(container.querySelectorAll("button"));
  return (
    buttons.find((item) =>
      item.textContent?.toLowerCase().includes(name.toLowerCase()),
    ) ?? null
  );
}

describe("AI growth optimization panel", () => {
  it("runs a mock optimization and supports per-platform decisions", async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <AIGrowthOptimizationPanel
        canEdit
        content={{
          firstImageSrc: "",
          html: "<p>Original body</p>",
          text: "Original body",
        }}
        projectId="project-1"
        selectedPlatforms={["wechat", "zhihu"]}
        title="Original title"
      />,
    );

    view.select("Goal", "views");
    view.select("Intensity", "aggressive");
    view.click(view.button("AI Optimize"));

    await act(async () => {
      await Promise.resolve();
    });

    expect(view.text()).toContain("Optimization ready");
    expect(view.text()).toContain("Original body");
    expect(view.text()).toContain("Optimized body");

    view.click(view.button("Apply WeChat"));
    await act(async () => {
      await Promise.resolve();
    });
    expect(view.text()).toContain("Applied");

    view.click(view.button("Reject Zhihu"));
    await act(async () => {
      await Promise.resolve();
    });
    expect(view.text()).toContain("Rejected");

    view.unmount();
  });

  it("prevents deciding old proposals while a rerun is pending", async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const createRunMock = vi.mocked(createAIGrowthOptimizationRun);
    createRunMock.mockRestore();

    const firstRun = await createRunMock("project-1", {
      goal: "views",
      intensity: "balanced",
      source_content: "Original body",
      target_platforms: ["wechat"],
      title: "Original title",
    });
    let resolveSecondRun: (value: typeof firstRun) => void = () => {};
    createRunMock.mockResolvedValueOnce(firstRun).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSecondRun = resolve;
      }),
    );

    const view = render(
      <AIGrowthOptimizationPanel
        canEdit
        content={{
          firstImageSrc: "",
          html: "<p>Original body</p>",
          text: "Original body",
        }}
        projectId="project-1"
        selectedPlatforms={["wechat"]}
        title="Original title"
      />,
    );

    view.click(view.button("AI Optimize"));
    await act(async () => {
      await Promise.resolve();
    });

    expect(view.button("Apply WeChat").disabled).toBe(false);

    view.click(view.button("AI Optimize"));

    const staleApplyButton = view.maybeButton("Apply WeChat");
    expect(staleApplyButton?.disabled ?? true).toBe(true);

    await act(async () => {
      resolveSecondRun(firstRun);
      await Promise.resolve();
    });

    view.unmount();
  });
});
