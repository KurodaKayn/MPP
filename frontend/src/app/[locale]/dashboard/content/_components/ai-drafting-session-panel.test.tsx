// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { AIDraftingSessionPanel } from "./ai-drafting-session-panel";

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

    expect(view.text()).toContain("Drafting sessions");
    expect(view.text()).toContain("No session selected");

    view.typeMessage("Improve the opening for WeChat");
    view.click(view.button("Send"));

    await act(async () => {
      await Promise.resolve();
    });

    expect(view.text()).toContain("Improve the opening for WeChat");
    expect(view.text()).toContain("I read the current project context");

    view.click(view.button("Events"));
    expect(view.text()).toContain("Read-only context");

    view.click(view.button("Artifacts"));
    expect(view.text()).toContain("Opening rewrite proposal");

    view.click(view.button("Archive"));

    await act(async () => {
      await Promise.resolve();
    });

    expect(view.text()).toContain("Archived");
    expect(view.button("Resume").disabled).toBe(false);
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

    view.click(view.button("New Session"));

    await act(async () => {
      await Promise.resolve();
    });

    view.click(view.button("Events"));
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

    expect(view.text()).toContain("Save the project before opening a session");
    expect(view.button("Send").disabled).toBe(true);

    view.unmount();
  });
});
