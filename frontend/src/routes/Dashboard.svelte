<script lang="ts">
  import { onMount } from "svelte";
  import { client, type Dashboard as Data } from "$lib/api";
  import { number, relativeTime } from "$lib/format";
  import ClassDistribution from "$components/ClassDistribution.svelte";
  let data = $state<Data | null>(null);
  let error = $state("");
  onMount(async () => {
    try {
      data = await client.dashboard();
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not reach the ledger";
    }
  });
</script>

<h1>The Enclosure</h1>
{#if error}<p class="error">{error}</p>{:else if !data}<p class="skeleton">
    Loading guild records…
  </p>{:else if data.rosterSize === 0}<section class="empty">
    <h2>The ledger awaits its first entry.</h2>
    <p>Run your first sync from <a href="/sync">Keeper Controls</a>.</p>
  </section>{:else}<section class="kpis">
    {#each [["Roster", number(data.rosterSize)], ["Max level", number(data.maxLevelMembers)], ["Active", number(data.activeMembers)], ["Avg M+", number(data.mythicPlus.averageRating)], ["Last sync", relativeTime(data.lastSync?.startedAt)]] as stat}<article
      >
        <small>{stat[0]}</small><strong>{stat[1]}</strong>
      </article>{/each}
  </section>
  <div class="grid">
    <section>
      <h2>Apelytics · Class ranks</h2>
      <ClassDistribution classes={data.classDistribution} />
      <div class="chips">
        {#each data.specDistribution as spec}<span>{spec.className} {spec.count}</span>{/each}
      </div>
    </section>
    <section>
      <h2>M+ Vanguard</h2>
      <table>
        <thead><tr><th>Ape</th><th>Rating</th><th>Best</th></tr></thead><tbody
          >{#each data.mythicPlus.top as row}<tr
              ><td>{row.name} <small>{row.realm}</small></td><td>{number(row.rating)}</td><td
                >+{row.bestKey}</td
              ></tr
            >{/each}</tbody
        >
      </table>
    </section>
    <section>
      <h2>Ape Watch · {data.staleCharacters.count}</h2>
      {#each data.staleCharacters.characters as row}<p>
          {row.name}
          <span class="stale">{row.syncedAt ? `${row.stalenessDays}d ago` : "never synced"}</span>
        </p>{/each}<a href="/roster?stale=true">Inspect stale roster →</a>
    </section>
    <section>
      <h2>Jungle Watch</h2>
      {#each data.notableChanges as row}<p>
          <strong>{row.name}</strong>
          <span class="up">↑ {row.ratingDelta ?? row.itemLevelDelta ?? 0}</span>
        </p>{:else}<p>No notable movement recorded yet.</p>{/each}
    </section>
  </div>{/if}
