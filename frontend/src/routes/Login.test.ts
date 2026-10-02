import { afterEach, describe, expect, it, vi } from "vitest";
import { flushSync, mount, unmount } from "svelte";
import Login from "$routes/Login.svelte";

describe("Login feedback", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    host?.remove();
    vi.unstubAllGlobals();
  });

  it("announces the sign-in transition and preserves the authentication endpoint", async () => {
    const href = vi.fn();
    vi.stubGlobal("location", {
      set href(value: string) {
        href(value);
      },
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(Login, { target: host });

    (host.querySelector("button") as HTMLButtonElement).click();
    flushSync();

    expect(href).toHaveBeenCalledWith("/api/auth/login");
    expect(host.querySelector('[role="status"]')?.textContent).toContain("Opening Battle.net");
    expect((host.querySelector("button") as HTMLButtonElement).disabled).toBe(true);
  });

  it("announces a redirect failure and leaves sign-in available", () => {
    vi.stubGlobal("location", {
      set href(_value: string) {
        throw new Error("navigation unavailable");
      },
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(Login, { target: host });

    (host.querySelector("button") as HTMLButtonElement).click();
    flushSync();

    expect(host.querySelector('[role="alert"]')?.textContent).toContain("could not be started");
    expect((host.querySelector("button") as HTMLButtonElement).disabled).toBe(false);
  });
});
