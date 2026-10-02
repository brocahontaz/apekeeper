import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, unmount } from "svelte";
import Roster from "./Roster.svelte";

const page = {
  items: [],
  total: 0,
  page: 1,
  pageSize: 25,
  classes: ["Mage", "Rogue"],
  specs: [],
};

describe("Roster URL state", () => {
  let host: HTMLDivElement;
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    host?.remove();
    history.replaceState({}, "", "/");
    vi.unstubAllGlobals();
  });

  it("restores filters and paging when browser navigation changes the URL", async () => {
    history.replaceState({}, "", "/roster?class=Mage&page=2");
    const requests: string[] = [];
    vi.stubGlobal("fetch", (input: string) => {
      requests.push(input);
      return Promise.resolve(new Response(JSON.stringify(page)));
    });
    host = document.createElement("div");
    document.body.append(host);
    component = mount(Roster, { target: host });

    await vi.waitFor(() => expect(requests).toHaveLength(1));
    expect(requests[0]).toContain("class=Mage");
    expect(requests[0]).toContain("page=2");

    history.pushState({}, "", "/roster?class=Rogue&page=3&sort=level");
    dispatchEvent(new PopStateEvent("popstate"));
    await vi.waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[1]).toContain("class=Rogue");
    expect(requests[1]).toContain("page=3");
    expect(requests[1]).toContain("sort=level");

    // jsdom does not reliably implement session-history traversal, so model
    // the browser's back target and notify the same popstate listener.
    history.pushState({}, "", "/roster?class=Mage&page=2");
    dispatchEvent(new PopStateEvent("popstate"));
    await vi.waitFor(() => expect(requests).toHaveLength(3));
    expect(requests[2]).toContain("class=Mage");
    expect(requests[2]).toContain("page=2");
  });
});
