// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import type { ReactElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAIGrowthOptimizationRun } from "@/lib/dashboard/api";
import { AIGrowthOptimizationPanel } from "./ai-growth-optimization-panel";
import zhCommon from "../../../../../../public/locales/zh/common.json";
import zhDashboard from "../../../../../../public/locales/zh/dashboard.json";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const i18nTestState = vi.hoisted(() => ({
  calls: [] as Array<{ key: string; options?: Record<string, unknown> }>,
  resources: undefined as Record<string, Record<string, unknown>> | undefined,
}));

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "zh",
  useTranslation: (_locale: string, namespace: string) => ({
    t: (key: string, options?: Record<string, unknown>) => {
      i18nTestState.calls.push({ key, options });

      let value: unknown = i18nTestState.resources?.[namespace];
      for (const segment of key.split(".")) {
        value =
          value && typeof value === "object"
            ? (value as Record<string, unknown>)[segment]
            : undefined;
      }

      if (typeof value !== "string") {
        return key;
      }

      return value.replace(/\{\{(\w+)\}\}/g, (_, token: string) =>
        String(options?.[token] ?? ""),
      );
    },
  }),
}));

i18nTestState.resources = {
  common: zhCommon,
  dashboard: zhDashboard,
};

const growthCopy = zhDashboard.content.aiGrowth;
const platformCopy = zhCommon.platforms;

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
    chooseSelect(labelText: string, value: string) {
      const trigger = findSelectTrigger(container, labelText);
      if (!trigger) {
        throw new Error(`Select trigger not found: ${labelText}`);
      }
      act(() => {
        trigger.click();
      });

      const option = Array.from(
        document.body.querySelectorAll<HTMLElement>("[role='option']"),
      ).find((item) => item.dataset.value === value);
      if (!option) {
        throw new Error(`Select option not found: ${value}`);
      }
      act(() => {
        option.click();
      });
    },
    nativeSelects() {
      return container.querySelectorAll("select");
    },
    selectTrigger(labelText: string) {
      return findSelectTrigger(container, labelText);
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

function findSelectTrigger(container: Element, labelText: string) {
  const labels = Array.from(container.querySelectorAll("label"));
  const label = labels.find((item) =>
    item.textContent?.toLowerCase().includes(labelText.toLowerCase()),
  );
  const controlId = label?.getAttribute("for");
  if (!controlId) {
    return null;
  }
  return container.querySelector<HTMLButtonElement>(`#${controlId}`);
}

describe("AI growth optimization panel", () => {
  beforeEach(() => {
    i18nTestState.calls = [];
  });

  it("reads panel copy from locale resources without English fallbacks", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <AIGrowthOptimizationPanel
        canEdit
        content={{
          firstImageSrc: "",
          html: "",
          text: "Original body",
        }}
        projectId="project-1"
        selectedPlatforms={["wechat"]}
        title="Original title"
      />,
    );

    expect(view.text()).toContain(growthCopy.title);
    expect(view.text()).toContain(growthCopy.description);
    expect(view.text()).toContain(growthCopy.goal);
    expect(view.nativeSelects()).toHaveLength(0);
    expect(view.selectTrigger(growthCopy.goal)).not.toBeNull();
    expect(view.selectTrigger(growthCopy.intensity)).not.toBeNull();
    expect(i18nTestState.calls).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          options: expect.objectContaining({ defaultValue: expect.anything() }),
        }),
      ]),
    );

    view.unmount();
  });

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

    view.chooseSelect(growthCopy.goal, "views");
    view.chooseSelect(growthCopy.intensity, "aggressive");
    view.click(view.button(growthCopy.optimize));

    await act(async () => {
      await Promise.resolve();
    });

    expect(view.text()).toContain(growthCopy.optimizationReady);
    expect(view.text()).toContain("Original body");
    expect(view.text()).toContain("Optimized body");

    view.click(
      view.button(
        growthCopy.applyLabel.replace("{{label}}", platformCopy.wechat),
      ),
    );
    await act(async () => {
      await Promise.resolve();
    });
    expect(view.text()).toContain(growthCopy.status.accepted);

    view.click(
      view.button(
        growthCopy.rejectLabel.replace("{{label}}", platformCopy.zhihu),
      ),
    );
    await act(async () => {
      await Promise.resolve();
    });
    expect(view.text()).toContain(growthCopy.status.rejected);

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

    view.click(view.button(growthCopy.optimize));
    await act(async () => {
      await Promise.resolve();
    });

    const applyWechat = growthCopy.applyLabel.replace(
      "{{label}}",
      platformCopy.wechat,
    );
    expect(view.button(applyWechat).disabled).toBe(false);

    view.click(view.button(growthCopy.optimize));

    const staleApplyButton = view.maybeButton(applyWechat);
    expect(staleApplyButton?.disabled ?? true).toBe(true);

    await act(async () => {
      resolveSecondRun(firstRun);
      await Promise.resolve();
    });

    view.unmount();
  });
});
