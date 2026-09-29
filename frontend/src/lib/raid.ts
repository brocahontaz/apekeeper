import type { Dashboard } from "./api";
export type RaidProgression = Dashboard["raidProgression"];
export type RaidRow = {
  raidName: string;
  difficulty: string;
  progress: number;
  totalBosses: number;
};
const CANONICAL_DIFFICULTIES = ["Raid Finder", "Normal", "Heroic", "Mythic"];
// Unknown difficulties sort after the canonical list, alphabetically as tiebreak.
function difficultyKey(difficulty: string): [number, string] {
  const index = CANONICAL_DIFFICULTIES.indexOf(difficulty);
  return [index === -1 ? CANONICAL_DIFFICULTIES.length : index, difficulty];
}
export function raidRows(raidProgression: RaidProgression): RaidRow[] {
  const rows: RaidRow[] = [];
  for (const raid of raidProgression ?? []) {
    for (const [difficulty, entry] of Object.entries(raid.difficulties ?? {})) {
      if (!entry || entry.totalBosses === 0) continue;
      rows.push({
        raidName: raid.raidName,
        difficulty,
        progress: entry.progress,
        totalBosses: entry.totalBosses,
      });
    }
  }
  return rows.sort((a, b) => {
    const [rankA, nameA] = difficultyKey(a.difficulty);
    const [rankB, nameB] = difficultyKey(b.difficulty);
    return a.raidName.localeCompare(b.raidName) || rankA - rankB || nameA.localeCompare(nameB);
  });
}
export function progressPct(row: Pick<RaidRow, "progress" | "totalBosses">): number {
  if (row.totalBosses === 0) return 0;
  return Math.min(100, Math.max(0, (row.progress / row.totalBosses) * 100));
}
