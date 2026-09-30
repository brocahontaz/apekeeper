import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { get } from "svelte/store";
import type { Theme } from "./colorScheme";

// colorScheme evaluates the initial theme at module load, so each case
// re-imports it after staging localStorage and matchMedia.
async function loadModule() {
  vi.resetModules();
  return await import("./colorScheme");
}

const prefersDark = vi.fn().mockReturnValue({ matches: true });
const prefersLight = vi.fn().mockReturnValue({ matches: false });

beforeEach(() => {
  localStorage.clear();
  document.documentElement.classList.remove("dark");
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("colorScheme", () => {
  it("falls back to the system preference when nothing is stored", async () => {
    vi.stubGlobal("matchMedia", prefersDark);
    const m = await loadModule();
    expect(get(m.theme)).toBe<Theme>("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("defaults to light when the system prefers light", async () => {
    vi.stubGlobal("matchMedia", prefersLight);
    const m = await loadModule();
    expect(get(m.theme)).toBe<Theme>("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("lets a persisted value win over the system preference", async () => {
    vi.stubGlobal("matchMedia", prefersDark);
    localStorage.setItem("apekeeper-theme", "light");
    const m = await loadModule();
    expect(get(m.theme)).toBe<Theme>("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("ignores invalid persisted values", async () => {
    vi.stubGlobal("matchMedia", prefersLight);
    localStorage.setItem("apekeeper-theme", "sepia");
    const m = await loadModule();
    expect(get(m.theme)).toBe<Theme>("light");
  });

  it("toggle persists and applies the opposite theme", async () => {
    vi.stubGlobal("matchMedia", prefersDark);
    const m = await loadModule();
    m.toggleTheme();
    expect(localStorage.getItem("apekeeper-theme")).toBe("light");
    expect(get(m.theme)).toBe<Theme>("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    m.toggleTheme();
    expect(localStorage.getItem("apekeeper-theme")).toBe("dark");
    expect(get(m.theme)).toBe<Theme>("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("tolerates a missing matchMedia (SSR/test safety)", async () => {
    const m = await loadModule();
    expect(get(m.theme)).toBe<Theme>("light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });
});
