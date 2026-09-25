import type { Character } from "./api";
export type Filters = { search?: string; class?: string; stale?: boolean };
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
  return query;
}
