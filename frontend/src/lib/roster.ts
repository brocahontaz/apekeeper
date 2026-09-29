import type { Character } from "./api";
export type Filters = {
  search?: string;
  class?: string;
  spec?: string;
  stale?: boolean;
  minLevel?: number;
  minRating?: number;
};
export function filterRoster(rows: Character[], f: Filters) {
  return rows.filter(
    (r) =>
      (!f.search || r.name.toLowerCase().includes(f.search.toLowerCase())) &&
      (!f.class || r.className === f.class) &&
      (!f.stale || r.stale),
  );
}

export type RosterSort =
  | "name"
  | "level"
  | "itemLevel"
  | "guildRank"
  | "realm"
  | "classSpec"
  | "mythicRating"
  | "syncedAt";
export type RosterDirection = "ascending" | "descending";

export function rosterQuery(
  filters: Filters & {
    page: number;
    pageSize: number;
    sort: RosterSort;
    direction: RosterDirection;
  },
) {
  const query = new URLSearchParams({
    page: String(filters.page),
    pageSize: String(filters.pageSize),
    sort: filters.sort,
    direction: filters.direction,
  });
  if (filters.search) query.set("search", filters.search);
  if (filters.class) query.set("class", filters.class);
  if (filters.stale) query.set("stale", "true");
  // Numeric thresholds are only sent when they carry a usable value: the
  // backend parses minLevel as any integer and minRating as any non-empty
  // float, so a zero or undefined level is meaningless while a defined
  // rating (including 0) is a real threshold.
  if (filters.spec) query.set("spec", filters.spec);
  if (filters.minLevel !== undefined && filters.minLevel > 0)
    query.set("minLevel", String(filters.minLevel));
  if (filters.minRating !== undefined && filters.minRating >= 0)
    query.set("minRating", String(filters.minRating));
  return query;
}
