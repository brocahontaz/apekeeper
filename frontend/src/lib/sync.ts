import type { SyncProgress, SyncRun } from "./api";
export type AppRole = "superadmin" | "admin" | "officer" | "member";
export function canTriggerSync(role: AppRole | undefined): boolean {
  return role === "admin" || role === "superadmin";
}
export function canViewRuns(role: AppRole | undefined): boolean {
  return canTriggerSync(role) || role === "officer";
}
// Live progress is visible to exactly the officers-and-above set that the
// run history it complements already allows.
export function canViewProgress(role: AppRole | undefined): boolean {
  return canViewRuns(role);
}
// phaseLabel maps a live sync phase onto expedition-log wording; unknown or
// missing phases fall back to the queued wording so the panel never shows a
// bare machine value.
export function phaseLabel(phase: string | undefined): string {
  switch (phase) {
    case "roster":
      return "Reading the roster";
    case "tier-anchor":
      return "Anchoring the tier";
    case "characters":
      return "Updating the crew";
    case "finalizing":
      return "Writing the ledger";
    default:
      return "Queued";
  }
}
// Terminal run statuses close the live view; a finished snapshot keeps its
// last counts visible with active=false.
export function progressTerminal(status: string): boolean {
  return status === "success" || status === "partial" || status === "failed";
}
// progressActive reports whether a run is still in flight; polling stops as
// soon as this turns false so the finished row can take over.
export function progressActive(p: Pick<SyncProgress, "active" | "status">): boolean {
  return p.active && !progressTerminal(p.status);
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
