<script lang="ts">
  import { onMount } from "svelte";
  import { client, type Dashboard as Data } from "$lib/api";
  import { number, relativeTime } from "$lib/format";
  import { raidRows, progressPct, type RaidRow } from "$lib/raid";
  import ClassDistribution from "$components/ClassDistribution.svelte";
  let data = $state<Data | null>(null);
  let error = $state("");
  let loading = $state(true);
  let generation = 0;
  const view = $derived(data as Data);
  async function load() {
    const request = ++generation;
    loading = true;
    error = "";
    try {
      const result = await client.dashboard();
      if (request === generation) data = result;
    } catch (e) {
      if (request === generation)
        error = e instanceof Error ? e.message : "Could not reach the ledger";
    } finally {
      if (request === generation) loading = false;
    }
  }
  onMount(() => {
    load();
    return () => generation++;
  });
  const difficultyColors: Record<string, string> = {
    "Raid Finder": "#cbb98d",
    Normal: "#9ebd72",
    Heroic: "#e8a33d",
    Mythic: "#f5c15e",
  };
  const difficultyColor = (difficulty: string) => difficultyColors[difficulty] ?? "#94a3a0";
  let raids = $derived.by((): [string, RaidRow[]][] => {
    if (!data) return [];
    const groups = new Map<string, RaidRow[]>();
    for (const row of raidRows(data.raidProgression)) {
      const existing = groups.get(row.raidName);
      if (existing) existing.push(row);
      else groups.set(row.raidName, [row]);
    }
    return [...groups.entries()];
  });
</script>

<h1>The Enclosure</h1>
{#if error}<section class="error-state" role="alert" aria-live="assertive">
    <p class="error">{error}</p>
    <button onclick={load}>Retry</button>
  </section>{/if}
{#if loading && !data && !error}<p class="skeleton" role="status" aria-live="polite">
    Loading guild records…
  </p>{:else if data && data.rosterSize === 0}<section class="empty" role="status">
    <h2>The ledger awaits its first entry.</h2>
    <p>Run your first sync from <a href="/sync">Keeper Controls</a>.</p>
  </section>{:else if data}<section class="kpis">
    {#each [["Roster", number(view.rosterSize)], ["Max level", number(view.maxLevelMembers)], ["Active", number(view.activeMembers)], ["Avg M+", number(view.mythicPlus.averageRating)], ["Last sync", relativeTime(view.lastSync?.startedAt)]] as stat}<article
      >
        <small>{stat[0]}</small><strong>{stat[1]}</strong>
      </article>{/each}
  </section>
  <div class="grid">
    <section>
      <h2>Apelytics · Class ranks</h2>
      <ClassDistribution classes={view.classDistribution} />
    </section>
    <section>
      <h2>M+ Vanguard</h2>
      <table>
        <thead><tr><th>Ape</th><th>Rating</th><th>Best</th></tr></thead><tbody
          >{#each view.mythicPlus.top as row}<tr
              ><td>{row.name} <small>{row.realm}</small></td><td>{number(row.rating)}</td><td
                >+{row.bestKey}</td
              ></tr
            >{/each}</tbody
        >
      </table>
      <p class="count">{number(view.mythicPlus.ratedCount)} rated characters</p>
    </section>
    <section>
      <h2>Ape Watch · {view.staleCharacters.count}</h2>
      {#each view.staleCharacters.characters as row}<p>
          {row.name}
          <span class="stale">{row.syncedAt ? `${row.stalenessDays}d ago` : "never synced"}</span>
        </p>{/each}<a href="/roster?stale=true">Inspect stale roster →</a>
    </section>
    <section>
      <h2>Jungle Watch</h2>
      {#each view.notableChanges as row}<p>
          <strong>{row.name}</strong>
          <span class="up">↑ {row.ratingDelta ?? row.itemLevelDelta ?? 0}</span>
        </p>{:else}<p>No notable movement recorded yet.</p>{/each}
    </section>
    <section>
      <h2>Guild trend</h2>
      {#each view.trends as trend}<p>
          {new Date(trend.capturedAt).toLocaleDateString()}: {trend.averageItemLevel.toFixed(1)} iLvl
          · {number(trend.averageRating)} M+ · {trend.staleCount} stale
          {#if trend.raidProgress.length}
            · {trend.raidProgress
              .map(
                (raid) =>
                  `${raid.raidName} ${raid.difficulty} ${raid.progress}/${raid.totalBosses}`,
              )
              .join(", ")}
          {/if}
        </p>{:else}<p>No retained snapshots to summarize yet.</p>{/each}
    </section>
    <section>
      <h2>Raid Progression</h2>
      {#each raids as [raidName, rows]}<div class="raid-group">
          <small>{raidName}</small>
          {#each rows as row}<div class="raid-row">
              <span>{row.difficulty}</span>
              <div class="bar-track">
                <i
                  style={`width:${progressPct(row)}%;background:${difficultyColor(row.difficulty)}`}
                ></i>
              </div>
              <b>{row.progress}/{row.totalBosses}</b>
            </div>{/each}
        </div>{:else}<p>No raid progress recorded yet.</p>{/each}
    </section>
  </div>{/if}
