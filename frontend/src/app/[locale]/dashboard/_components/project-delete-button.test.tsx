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
      return container.textContent ?? "";
    },
    unmount() {
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

describe("ProjectDeleteButton", () => {
  it("renders an accessible trash icon button", () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    const onDelete = vi.fn();
    const view = render(
      <ProjectDeleteButton label="Delete project" onDelete={onDelete} />,
    );

    act(() => {
      view.button().click();
    });

    expect(view.button().type).toBe("button");
    expect(view.text()).toContain("Delete project");
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
