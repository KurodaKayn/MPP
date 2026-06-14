// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import type { ReactElement } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AIDraftingSessionPanel } from "./ai-drafting-session-panel";
import zhDashboard from "../../../../../../public/locales/zh/dashboard.json";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

const i18nTestState = vi.hoisted(() => ({
  calls: [] as Array<{ key: string; options?: Record<string, unknown> }>,
  dashboard: undefined as Record<string, unknown> | undefined,
}));

vi.mock("@/lib/i18n/client", () => ({
  useAppLocale: () => "zh",
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      i18nTestState.calls.push({ key, options });

      let value: unknown = i18nTestState.dashboard;
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

i18nTestState.dashboard = zhDashboard;
const draftingCopy = zhDashboard.content.draftingSession;

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
        if (target instanceof HTMLElement) {
          target.click();
        } else {
          target.dispatchEvent(new MouseEvent("click", { bubbles: true }));
        }
      });
    },
    text() {
      return container.textContent ?? "";
    },
    textarea() {
      const textarea = container.querySelector("textarea");
      if (!textarea) {
        throw new Error("Textarea not found");
      }
      return textarea;
    },
    typeMessage(value: string) {
      const textarea = this.textarea();
      act(() => {
        const setter = Object.getOwnPropertyDescriptor(
          HTMLTextAreaElement.prototype,
          "value",
        )?.set;
        setter?.call(textarea, value);
        textarea.dispatchEvent(new Event("input", { bubbles: true }));
      });
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

describe("AI drafting session panel", () => {
  beforeEach(() => {
    i18nTestState.calls = [];
  });

  it("creates a mock session, appends assistant events, and archives it", async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <AIDraftingSessionPanel
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

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(view.text()).toContain(draftingCopy.title);
    expect(view.text()).toContain(draftingCopy.description);
    expect(view.text()).toContain(draftingCopy.empty.noSession);

    view.typeMessage("Improve the opening for WeChat");
    view.click(view.button(draftingCopy.actions.send));

    await act(async () => {
      await Promise.resolve();
    });

    expect(view.text()).toContain("Improve the opening for WeChat");
    expect(view.text()).toContain("I read the current project context");

    view.click(view.button(draftingCopy.tabs.events));
    expect(view.text()).toContain("Read-only context");

    view.click(view.button(draftingCopy.tabs.artifacts));
    expect(view.text()).toContain("Opening rewrite proposal");

    view.click(view.button(draftingCopy.actions.archive));

    await act(async () => {
      await Promise.resolve();
    });

    expect(view.text()).toContain(draftingCopy.state.archived);
    expect(view.button(draftingCopy.actions.resume).disabled).toBe(false);
    view.unmount();
  });

  it("reads drafting copy from dashboard locale resources without default values", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <AIDraftingSessionPanel
        canEdit
        content={{
          firstImageSrc: "",
          html: "",
          text: "Draft body",
        }}
        selectedPlatforms={["wechat"]}
        title="Draft title"
      />,
    );

    expect(view.text()).toContain(draftingCopy.title);
    expect(view.text()).toContain(draftingCopy.unsavedProject);
    expect(i18nTestState.calls).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          options: expect.objectContaining({ defaultValue: expect.anything() }),
        }),
      ]),
    );

    view.unmount();
  });

  it("renders assistant, status, read-only context, and compact boundary events", async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <AIDraftingSessionPanel
        canEdit
        content={{
          firstImageSrc: "",
          html: "<p>Original body</p>",
          text: "Original body",
        }}
        projectId="project-compact"
        selectedPlatforms={["wechat"]}
        title="Compact title"
      />,
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    view.click(view.button(draftingCopy.actions.newSession));

    await act(async () => {
      await Promise.resolve();
    });

    view.click(view.button(draftingCopy.tabs.events));
    expect(view.text()).toContain("Assistant text");
    expect(view.text()).toContain("Read-only context");
    expect(view.text()).toContain("Status update");
    expect(view.text()).toContain("Compact boundary");

    view.unmount();
  });

  it("disables sending until a persisted project exists", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <AIDraftingSessionPanel
        canEdit
        content={{
          firstImageSrc: "",
          html: "",
          text: "Draft body",
        }}
        selectedPlatforms={["wechat"]}
        title="Draft title"
      />,
    );

    expect(view.text()).toContain(draftingCopy.unsavedProject);
    expect(view.button(draftingCopy.actions.send).disabled).toBe(true);

    view.unmount();
  });
});
