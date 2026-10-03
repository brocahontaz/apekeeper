import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";

const user = { id: 1, displayName: "Keeper", battletag: "keeper#1", appRole: "admin" };
const character = {
  id: 7,
  name: "Alpha",
  realm: "Area 52",
  classId: 1,
  className: "Warrior",
  specId: 71,
  specName: "Arms",
  level: 80,
  itemLevel: 620,
  mythicRating: 2500,
  bestKeyLevel: 12,
  stale: false,
};
const secondCharacter = {
  ...character,
  id: 8,
  name: "Bravo",
  className: "Mage",
  specName: "Arcane",
};

async function doubles(page: import("@playwright/test").Page) {
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith("/api/")) return route.continue();
    if (url.pathname === "/api/auth/me") return route.fulfill({ json: user });
    if (url.pathname === "/api/guilds")
      return route.fulfill({
        json: [
          {
            id: 1,
            slug: "apes",
            name: "Apes",
            realm: "Area 52",
            region: "us",
            role: "admin",
            selected: true,
          },
        ],
      });
    if (url.pathname === "/api/roster/export")
      return route.fulfill({
        headers: {
          "Content-Type": "text/csv",
          "Content-Disposition": "attachment; filename=roster.csv",
        },
        body: "name,realm\nAlpha,Area 52\n",
      });
    if (url.pathname === "/api/roster")
      return route.fulfill({
        json: {
          items:
            url.searchParams.get("search") === "Alpha" ? [character] : [character, secondCharacter],
          total: url.searchParams.get("search") === "Alpha" ? 1 : 2,
          page: 1,
          pageSize: 25,
          classes: ["Warrior"],
          specs: ["Arms"],
        },
      });
    if (url.pathname === "/api/characters/Alpha")
      return route.fulfill({
        json: { ...character, mythicPlus: [], raidProgression: [], snapshots: [] },
      });
    if (url.pathname === "/api/characters/Alpha/history")
      return route.fulfill({
        json: {
          from: "2026-01-01T00:00:00Z",
          to: "2026-01-02T00:00:00Z",
          retentionDays: 90,
          snapshots: [],
          comparison: null,
        },
      });
    if (url.pathname === "/api/dashboard")
      return route.fulfill({
        json: {
          rosterSize: 1,
          maxLevelMembers: 1,
          activeMembers: 1,
          classDistribution: [],
          mythicPlus: { top: [], averageRating: 0, ratedCount: 0 },
          raidProgression: [],
          staleCharacters: { count: 0, characters: [] },
          notableChanges: [],
          trends: [],
        },
      });
    if (url.pathname === "/api/sync/progress")
      return route.fulfill({
        json: {
          active: false,
          runId: 9,
          status: "success",
          phase: "finalizing",
          total: 1,
          updated: 1,
          failed: 0,
          startedAt: "2026-01-01T00:00:00Z",
          dryRun: false,
        },
      });
    if (url.pathname === "/api/sync/runs")
      return route.fulfill({
        json: [
          {
            id: 9,
            guildId: 1,
            startedAt: "2026-01-01T00:00:00Z",
            finishedAt: null,
            trigger: "manual",
            status: "running",
            total: 1,
            updated: 0,
            failed: 0,
            errorSummary: "",
            detail: {},
          },
        ],
      });
    return route.fulfill({ json: {} });
  });
}

test("deterministic session and login guard", async ({ page }) => {
  await page.route("**/api/auth/me", (route) =>
    route.fulfill({ status: 401, json: { code: "unauthenticated", message: "unauthenticated" } }),
  );
  await page.goto("/");
  await expect(page.getByRole("button", { name: /sign in/i })).toBeVisible();
});

test("roster, detail, theme, export, and sync status use test doubles", async ({ page }) => {
  await doubles(page);
  await page.context().route(/\/api\/roster\/export/, (route) =>
    route.fulfill({
      headers: {
        "Content-Type": "text/csv",
        "Content-Disposition": "attachment; filename=roster.csv",
      },
      body: "name,realm\nAlpha,Area 52\n",
    }),
  );
  await page.goto("/roster");
  await expect(page.getByRole("heading", { name: "Roster" })).toBeVisible();
  await page.getByLabel("Search roster").fill("Alpha");
  await expect(page).toHaveURL(/search=Alpha/);
  await page.getByRole("button", { name: "Toggle dark mode" }).click();
  await expect(page.getByRole("button", { name: "Toggle dark mode" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(page.getByText("Showing 1 of 1 matching apes")).toBeVisible();
  await expect(page.getByText("Export CSV")).toHaveAttribute("href", /roster\/export/);
  const downloadPromise = page.waitForEvent("download");
  await page.getByText("Export CSV").click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("roster.csv");
  expect(await readFile((await download.path())!, "utf8")).toContain("Alpha,Area 52");
  await page.getByRole("link", { name: "Alpha" }).click();
  await expect(page.getByRole("heading", { name: "Alpha" })).toBeVisible();
  await page.goto("/sync");
  await expect(page.getByText(/success/i)).toBeVisible();
});
