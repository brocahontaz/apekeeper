<script lang="ts">
  import { onMount } from "svelte";
  import { client, type Character, type History, type MythicRun, type Snapshot } from "$lib/api";
  import HistoryCharts from "$components/HistoryCharts.svelte";
  import { classColor } from "$lib/theme";
  import { relativeTime, raceLabel } from "$lib/format";
  let { name }: { name: string } = $props();
  let character = $state<Character | null>(null);
  let error = $state("");
  let history = $state<History | null>(null);
  let historyError = $state("");
  let compareFrom = $state("");
  let compareTo = $state("");
  const validNumber = (value: unknown): value is number =>
    typeof value === "number" && Number.isFinite(value);
  function isHistorySnapshot(value: unknown): value is Snapshot {
    if (!value || typeof value !== "object") return false;
    const snapshot = value as Partial<Snapshot>;
    return (
      validNumber(snapshot.id) &&
      typeof snapshot.capturedAt === "string" &&
      !Number.isNaN(Date.parse(snapshot.capturedAt)) &&
      validNumber(snapshot.itemLevel) &&
      validNumber(snapshot.mythicRating) &&
      validNumber(snapshot.bestKeyLevel)
    );
  }
  function validSnapshots(value: unknown): Snapshot[] {
    return Array.isArray(value) ? value.filter(isHistorySnapshot) : [];
  }
  let selectableSnapshots = $derived(validSnapshots(history?.snapshots));
  let tableSnapshots = $derived(validSnapshots(history?.snapshots ?? character?.snapshots));
  type MythicRow = {
    season?: string;
    overallRating: number;
    bestKeyLevel: number;
    runs?: { best?: MythicRun[]; recent?: MythicRun[] };
  };
  onMount(async () => {
    try {
      character = await client.character(name);
    } catch (e) {
      error = e instanceof Error ? e.message : "Character unavailable";
      return;
    }
    await loadHistory();
  });
  async function loadHistory(query = "") {
    if (!character) return;
    historyError = "";
    try {
      const result = await client.history(character.id, query);
      if (!Array.isArray(result.snapshots)) throw new Error("History data is malformed");
      history = result;
    } catch (e) {
      historyError = e instanceof Error ? e.message : "History unavailable";
    }
  }
  async function compare() {
    if (!character || !compareFrom || !compareTo) return;
    await loadHistory(
      `?compareFrom=${encodeURIComponent(compareFrom)}&compareTo=${encodeURIComponent(compareTo)}`,
    );
  }
  // The season payload repeats a dungeon once per timed and untimed attempt;
  // keep only the highest score per dungeon for the best-runs view.
  function bestByDungeon(runs: MythicRun[] | undefined): MythicRun[] {
    const top = new Map<string, MythicRun>();
    for (const r of runs ?? []) {
      const kept = top.get(r.dungeon);
      if (!kept || r.score > kept.score) top.set(r.dungeon, r);
    }
    return [...top.values()].sort((a, b) => b.score - a.score);
  }
</script>

<a href="/roster">← Roster</a>{#if error}<p class="error">{error}</p>{:else if !character}<p
    class="skeleton"
  >
    Opening ape file…
  </p>{:else}<header class="character">
    <div class="avatar">
      {#if character.avatarUrl}
        <img src={character.avatarUrl} alt={character.name} />
      {:else}
        {character.name[0]}
      {/if}
    </div>
    <div>
      <h1 style={`color:${classColor(character.classId)}`}>{character.name}</h1>
      <div class="character-stats">
        <span><small>Realm</small>{character.realm}</span><span
          ><small>Class</small>{character.className}</span
        ><span><small>Specialization</small>{character.specName}</span><span
          ><small>Race</small>{raceLabel(character.raceName, character.gender)}</span
        ><span><small>Level</small>{character.level}</span><span
          ><small>Item level</small>{character.itemLevel}</span
        >
      </div>
      <span class:stale={character.stale}
        >{character.stale ? "Needs attention" : `Synced ${relativeTime(character.syncedAt)}`}</span
      >
    </div>
  </header>
  <div class="grid">
    <section>
      <h2>Mythic+ stats</h2>
      {#each (character.mythicPlus as MythicRow[]) ?? [] as m}<div class="progression-row">
          <strong>{m.season || "Current season"}</strong><span
            >Rating <b>{Math.round(m.overallRating).toLocaleString()}</b></span
          ><span>Best key <b>+{m.bestKeyLevel}</b></span>
        </div>
        {#if m.runs?.best?.length}<div class="runs-group">
            <small>Best runs</small>
            {#each bestByDungeon(m.runs?.best) as r}<div class="progression-row">
                <strong>{r.dungeon}</strong><span>+{r.level}</span><span
                  >Score <b>{Math.round(r.score).toLocaleString()}</b></span
                ><span>{r.timed ? "timed" : "over time"}</span>
              </div>{/each}
          </div>
        {/if}
        {#if m.runs?.recent?.length}<div class="runs-box">
            <small>Latest runs</small>
            {#each m.runs.recent as r}<div class="progression-row">
                <strong>{r.dungeon}</strong><span>+{r.level}</span><span
                  >{r.timed ? "timed" : "over time"}</span
                ><span>{relativeTime(new Date(r.completedAt))}</span>
              </div>{/each}
          </div>
        {/if}{:else}<p>No keystones recorded.</p>{/each}
    </section>
    <section>
      <h2>Raid stats</h2>
      {#each (character.raidProgression as any[]) ?? [] as r}<div class="progression-row">
          <strong>{r.raidName || "Current tier"}</strong><span>{r.difficulty}</span><span
            ><progress value={r.progress} max={r.totalBosses}></progress>
            {r.progress}/{r.totalBosses}</span
          >
        </div>{:else}<p>No progression in the current tier.</p>{/each}
    </section>
    <section>
      <h2>Progression history</h2>
      {#if historyError}<p class="error">{historyError}</p>{:else if !history}<p class="skeleton">
          Loading expedition history…
        </p>{:else}
        <p class="count">
          Snapshots are retained for {history.retentionDays} days. Older records are unavailable.
        </p>
        <HistoryCharts snapshots={history.snapshots} />
        {#if selectableSnapshots.length > 1}
          <form
            onsubmit={(event) => {
              event.preventDefault();
              compare();
            }}
            aria-label="Compare progression snapshots"
          >
            <label
              >Earlier snapshot <select bind:value={compareFrom}
                >{#each selectableSnapshots as s}<option value={s.id}
                    >{new Date(s.capturedAt).toLocaleString()}</option
                  >{/each}</select
              ></label
            >
            <label
              >Later snapshot <select bind:value={compareTo}
                >{#each selectableSnapshots as s}<option value={s.id}
                    >{new Date(s.capturedAt).toLocaleString()}</option
                  >{/each}</select
              ></label
            >
            <button type="submit" disabled={!compareFrom || !compareTo || compareFrom === compareTo}
              >Compare</button
            >
          </form>
          {#if history.comparison}<p class="count">
              Comparison: {history.comparison.itemLevelDelta >= 0
                ? "+"
                : ""}{history.comparison.itemLevelDelta.toFixed(1)} iLvl, {history.comparison
                .mythicRatingDelta >= 0
                ? "+"
                : ""}{Math.round(history.comparison.mythicRatingDelta)} rating, {history.comparison
                .bestKeyLevelDelta >= 0
                ? "+"
                : ""}{history.comparison.bestKeyLevelDelta} key levels.
            </p>{/if}
        {:else}<p class="count">A second snapshot is needed for comparison.</p>{/if}
      {/if}
      <table>
        <thead><tr><th>Date</th><th>iLvl</th><th>Rating</th><th>Key</th></tr></thead><tbody
          >{#each tableSnapshots as s}<tr
              ><td>{new Date(s.capturedAt).toLocaleDateString()}</td><td>{s.itemLevel}</td><td
                >{s.mythicRating}</td
              ><td>+{s.bestKeyLevel}</td></tr
            >{/each}</tbody
        >
      </table>
    </section>
  </div>{/if}
