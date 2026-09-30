import { get, writable } from "svelte/store";

// Color scheme (light/dark) handling: the stored choice wins, otherwise the
// system preference applies. All browser access is guarded so importing this
// module stays safe outside a browser (SSR or tests).

export type Theme = "light" | "dark";

const STORAGE_KEY = "apekeeper-theme";

function readStoredTheme(): Theme | null {
  try {
    const value = localStorage.getItem(STORAGE_KEY);
    return value === "light" || value === "dark" ? value : null;
  } catch {
    return null;
  }
}

function systemPrefersDark(): boolean {
  try {
    return typeof matchMedia === "function" && matchMedia("(prefers-color-scheme: dark)").matches;
  } catch {
    return false;
  }
}

function initialTheme(): Theme {
  return readStoredTheme() ?? (systemPrefersDark() ? "dark" : "light");
}

function applyDocumentClass(value: Theme): void {
  try {
    document.documentElement.classList.toggle("dark", value === "dark");
  } catch {
    // No document to update (SSR or tests); the store still tracks the choice.
  }
}

const initial = initialTheme();
export const theme = writable<Theme>(initial);
applyDocumentClass(initial);

export function setTheme(value: Theme): void {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // Persistence is best effort; the toggle still applies immediately.
  }
  theme.set(value);
  applyDocumentClass(value);
}

export function toggleTheme(): void {
  setTheme(get(theme) === "dark" ? "light" : "dark");
}
