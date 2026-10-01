import { describe, it, expect } from "vitest";
import { rosterExportQuery } from "./roster";

describe("rosterExportQuery", () => {
  it("omits empty filters", () =>
    expect(
      rosterExportQuery({
        search: "",
        class: "",
        spec: "",
        stale: false,
        minLevel: 0,
        sort: "guildRank",
        direction: "ascending",
      }).toString(),
    ).toBe("sort=guildRank&direction=ascending"));

  it("carries every active filter and the ordering", () =>
    expect(
      rosterExportQuery({
        search: "alp",
        class: "Mage",
        spec: "Arcane",
        minLevel: 70,
        minRating: 1000,
        stale: true,
        sort: "mythicRating",
        direction: "descending",
      }).toString(),
    ).toBe(
      "sort=mythicRating&direction=descending&search=alp&class=Mage&stale=true&spec=Arcane&minLevel=70&minRating=1000",
    ));

  it("never emits pagination parameters", () => {
    const query = rosterExportQuery({ sort: "name", direction: "ascending" });
    expect(query.has("page")).toBe(false);
    expect(query.has("pageSize")).toBe(false);
  });

  it("sends a zero rating threshold but drops a zero level", () =>
    expect(
      rosterExportQuery({ minRating: 0, sort: "level", direction: "descending" }).toString(),
    ).toBe("sort=level&direction=descending&minRating=0"));
});
