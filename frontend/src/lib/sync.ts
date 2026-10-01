import type { SyncRun } from "./api";
export type AppRole = "superadmin" | "admin" | "officer" | "member";
export function canTriggerSync(role: AppRole | undefined): boolean {
  return role === "admin" || role === "superadmin";
}
export function canViewRuns(role: AppRole | undefined): boolean {
  return canTriggerSync(role) || role === "officer";
}
export type FailureDetail = { name: string; error: string };
// detail is a jsonb map of character name -> error string; be defensive about
// anything else the API might hand back.
export function failureDetails(run: Pick<SyncRun, "detail">): FailureDetail[] {
  const detail = run?.detail;
  if (detail === null || typeof detail !== "object" || Array.isArray(detail)) return [];
  return Object.entries(detail as Record<string, unknown>)
    .filter((entry): entry is [string, string] => typeof entry[1] === "string")
    .map(([name, error]) => ({ name, error }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

export type NotifyChip = { label: string; kind: "up" | "stale" | "error" };
// Maps a run's additive notifyStatus onto a chip beside the run status;
// unknown or never-recorded values (null/absent) render nothing at all.
export function notifyStatusChip(status: string | null | undefined): NotifyChip | null {
  switch (status) {
    case "sent":
      return { label: "notify sent", kind: "up" };
    case "skipped":
      return { label: "notify skipped", kind: "stale" };
    case "failed":
      return { label: "notify failed", kind: "error" };
    default:
      return null;
  }
}
