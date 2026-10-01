import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { flushSync, mount, unmount } from "svelte";
import ThemeToggle from "./ThemeToggle.svelte";
import { setTheme } from "$lib/colorScheme";

describe("ThemeToggle", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;

  beforeEach(() => {
    host = document.createElement("div");
    document.body.appendChild(host);
  });

  afterEach(() => {
    unmount(component);
    host.remove();
    setTheme("light");
  });

  it("reflects the active theme in aria-pressed and the glyph", () => {
    setTheme("dark");
    component = mount(ThemeToggle, { target: host });
    const button = host.querySelector("button")!;
    expect(button.getAttribute("aria-pressed")).toBe("true");
    expect(button.getAttribute("aria-label")).toBe("Toggle dark mode");
    expect(button.textContent).toBe("☀");
    setTheme("light");
    flushSync();
    expect(button.getAttribute("aria-pressed")).toBe("false");
    expect(button.textContent).toBe("☾");
  });
});
