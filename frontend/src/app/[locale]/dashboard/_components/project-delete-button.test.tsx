// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import { ProjectDeleteButton } from "./project-delete-button";

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

function render(element: React.ReactElement) {
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(element);
  });

  return {
    button() {
      const button = container.querySelector("button");
      if (!button) {
        throw new Error("button not found");
      }
      return button;
    },
    text() {
      return document.body.textContent ?? "";
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

function waitForUpdates() {
  return new Promise((resolve) => window.setTimeout(resolve, 0));
}

function buttonByText(text: string) {
  const button = Array.from(document.body.querySelectorAll("button")).find(
    (item) => item.textContent?.trim() === text,
  );
  if (!button) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
}

describe("ProjectDeleteButton", () => {
  it("opens a confirmation dialog before deleting", async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const onDelete = vi.fn();
    const view = render(
      <ProjectDeleteButton
        confirmCancelLabel="Cancel"
        confirmDescription="Delete First project?"
        confirmSubmitLabel="Delete"
        confirmTitle="Delete project"
        label="Delete project"
        onDelete={onDelete}
      />,
    );

    await act(async () => {
      view.button().click();
      await waitForUpdates();
    });

    expect(view.button().type).toBe("button");
    expect(view.text()).toContain("Delete project");
    expect(view.text()).toContain("Delete First project?");
    expect(onDelete).not.toHaveBeenCalled();

    await act(async () => {
      buttonByText("Delete").click();
      await waitForUpdates();
    });

    expect(onDelete).toHaveBeenCalledOnce();

    view.unmount();
  });

  it("disables the button while deleting", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const view = render(
      <ProjectDeleteButton
        isDeleting
        label="Deleting project"
        onDelete={vi.fn()}
      />,
    );

    expect(view.button().disabled).toBe(true);
    expect(view.text()).toContain("Deleting project");

    view.unmount();
  });
});
