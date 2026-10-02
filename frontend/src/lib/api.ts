import { sessionExpired, currentUser } from "./stores";
import { navigate } from "./router";
export type User = {
  id: number;
  displayName: string;
  battletag: string;
  appRole: "superadmin" | "admin" | "officer" | "member";
};
export type Character = {
  id: number;
  name: string;
  realm: string;
  classId: number;
  className: string;
  specId: number;
  specName: string;
  role?: string;
  level: number;
  itemLevel: number;
  guildRank?: number;
  raceName?: string;
  gender?: string;
  avatarUrl?: string;
  mythicRating: number;
  bestKeyLevel: number;
  syncedAt?: string;
  stale: boolean;
  [key: string]: unknown;
};
export type Snapshot = {
  id: number;
  characterId: number;
  capturedAt: string;
  itemLevel: number;
  mythicRating: number;
  bestKeyLevel: number;
  // Snapshots normally retain Blizzard's raid object, while snapshots with no
  // raid data are serialized by the backend as an empty JSON array.
  raidProgress?: Record<string, unknown> | [];
};
export type History = {
  from: string;
  to: string;
  retentionDays: number;
  snapshots: Snapshot[];
  comparison: {
    from: Snapshot;
    to: Snapshot;
    itemLevelDelta: number;
    mythicRatingDelta: number;
    bestKeyLevelDelta: number;
  } | null;
};
export type MythicRun = {
  dungeon: string;
  level: number;
  score: number;
  timed: boolean;
  completedAt: number;
};
export type RosterPage = {
  items: Character[];
  total: number;
  page: number;
  pageSize: number;
  classes: string[];
  specs: string[];
};
export type SyncRun = {
  id: number;
  guildId: number;
  startedAt: string;
  finishedAt: string | null;
  trigger: string;
  status: string;
  notifyStatus?: string | null;
  total: number;
  updated: number;
  failed: number;
  errorSummary: string;
  detail: unknown;
};
export type SyncProgress = {
  active: boolean;
  runId: number;
  status: string;
  phase: string;
  total: number;
  updated: number;
  failed: number;
  startedAt: string;
  dryRun: boolean;
};
export type OfficerQueueItem = {
  characterId?: number;
  syncRunId?: number;
  name: string;
  reason: string;
  tags: string[];
  syncedAt?: string;
};
export type OfficerActivity = {
  id: number;
  actor: string;
  action: string;
  targetId?: number;
  createdAt: string;
  changes: Record<string, unknown>;
};
export type OfficerCharacterMetadata = {
  id: number;
  note: string;
  lifecycleStatus: string;
  tags: string[];
  reviewedAt?: string;
  noteAuthor?: string;
};
export type Dashboard = {
  rosterSize: number;
  maxLevelMembers: number;
  activeMembers: number;
  classDistribution: {
    classId: number;
    className: string;
    count: number;
    specs: { name: string; count: number }[];
  }[];
  mythicPlus: {
    top: { name: string; realm: string; rating: number; bestKey: number }[];
    averageRating: number;
    ratedCount: number;
  };
  raidProgression: {
    raidName: string;
    difficulties: Record<string, { progress: number; totalBosses: number }>;
  }[];
  staleCharacters: {
    count: number;
    characters: { name: string; realm: string; syncedAt?: string; stalenessDays: number }[];
  };
  lastSync?: { status?: string; startedAt?: string };
  notableChanges: { name: string; ratingDelta?: number; itemLevelDelta?: number }[];
  trends: {
    capturedAt: string;
    averageItemLevel: number;
    averageRating: number;
    staleCount: number;
    raidProgress: { raidName: string; difficulty: string; progress: number; totalBosses: number }[];
  }[];
};
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { credentials: "same-origin", ...init });
  if (response.status === 401) {
    sessionExpired.set(true);
    currentUser.set(null);
    if (typeof window !== "undefined" && location.pathname !== "/login") navigate("/login");
  }
  if (!response.ok) {
    let message = response.statusText;
    try {
      message = (await response.json()).error || message;
    } catch {}
    throw new ApiError(response.status, message);
  }
  return response.status === 204 ? (undefined as T) : response.json();
}
export const client = {
  me: () => api<User>("/api/auth/me"),
  dashboard: () => api<Dashboard>("/api/dashboard"),
  roster: (q = "") => api<RosterPage>(`/api/roster${q}`),
  character: (name: string) => api<Character>(`/api/characters/${encodeURIComponent(name)}`),
  history: (id: number, q = "") => api<History>(`/api/characters/${id}/history${q}`),
  runs: () => api<SyncRun[]>("/api/sync/runs"),
  progress: () => api<SyncProgress>("/api/sync/progress"),
  sync: () => api<{ runId: number }>("/api/sync/run", { method: "POST" }),
  dryRun: () => api<{ runId: number }>("/api/sync/dry-run", { method: "POST" }),
  retry: (id: number) => api<{ runId: number }>(`/api/sync/runs/${id}/retry`, { method: "POST" }),
  cancel: () => api<{ cancelled: boolean }>("/api/sync/cancel", { method: "POST" }),
  officerQueue: (reason = "") =>
    api<{ items: OfficerQueueItem[] }>(
      `/api/officer/queue${reason ? `?reason=${encodeURIComponent(reason)}` : ""}`,
    ),
  officerActivity: () => api<OfficerActivity[]>("/api/officer/activity"),
  officerCharacter: (id: number) => api<OfficerCharacterMetadata>(`/api/officer/characters/${id}`),
  bulkTags: (characterIds: number[], addTags: string[], removeTags: string[]) =>
    api<{ updated: number }>("/api/officer/bulk-tags", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ characterIds, addTags, removeTags, confirm: true }),
    }),
  completeReview: (characterIds: number[], syncRunIds: number[]) =>
    api<{ completed: number }>("/api/officer/queue/complete", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ characterIds, syncRunIds, confirm: true }),
    }),
  updateOfficerCharacter: (id: number, note: string, lifecycleStatus: string, tags: string[]) =>
    api<{ updated: boolean }>(`/api/officer/characters/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ note, lifecycleStatus, tags }),
    }),
  logout: () => api<void>("/api/auth/logout", { method: "POST" }),
};
