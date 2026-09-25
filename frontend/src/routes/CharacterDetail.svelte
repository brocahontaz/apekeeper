<script lang="ts">
  import { onMount } from "svelte";
  import { client, type Character } from "$lib/api";
  import { classColor } from "$lib/theme";
  import { relativeTime } from "$lib/format";
  let { name }: { name: string } = $props();
  let character = $state<Character | null>(null);
  let error = $state("");
  onMount(async () => {
    try {
      character = await client.character(name);
    } catch (e) {
      error = e instanceof Error ? e.message : "Character unavailable";
    }
  });
</script>

<a href="/roster">← Roster</a>{#if error}<p class="error">{error}</p>{:else if !character}<p
    class="skeleton"
  >
    Opening ape file…
  </p>{:else}<header class="character">
    <div class="avatar">{character.name[0]}</div>
    <div>
      <h1 style={`color:${classColor(character.classId)}`}>{character.name}</h1>
      <div class="character-stats">
        <span><small>Realm</small>{character.realm}</span><span
          ><small>Class</small>{character.className}</span
        ><span><small>Specialization</small>{character.specName}</span><span
          ><small>Level</small>{character.level}</span
        ><span><small>Item level</small>{character.itemLevel}</span>
      </div>
      <span class:stale={character.stale}
        >{character.stale ? "Needs attention" : `Synced ${relativeTime(character.syncedAt)}`}</span
      >
    </div>
  </header>
  <div class="grid">
    <section>
      <h2>Mythic+ stats</h2>
      {#each (character.mythicPlus as any[]) ?? [] as m}<div class="progression-row">
          <strong>{m.season}</strong><span
            >Rating <b>{Math.round(m.overallRating).toLocaleString()}</b></span
          ><span>Best key <b>+{m.bestKeyLevel}</b></span>
        </div>{:else}<p>No keystones recorded.</p>{/each}
    </section>
    <section>
      <h2>Raid stats</h2>
      {#each (character.raidProgression as any[]) ?? [] as r}<div class="progression-row">
          <strong>{r.raidName}</strong><span>{r.difficulty}</span><span
            ><progress value={r.progress} max={r.totalBosses}></progress>
            {r.progress}/{r.totalBosses}</span
          >
        </div>{:else}<p>No raid records.</p>{/each}
    </section>
    <section>
      <h2>Progression history</h2>
      <table>
        <thead><tr><th>Date</th><th>iLvl</th><th>Rating</th><th>Key</th></tr></thead><tbody
          >{#each (character.snapshots as any[]) ?? [] as s}<tr
              ><td>{new Date(s.capturedAt).toLocaleDateString()}</td><td>{s.itemLevel}</td><td
                >{s.mythicRating}</td
              ><td>+{s.bestKeyLevel}</td></tr
            >{/each}</tbody
        >
      </table>
    </section>
  </div>{/if}
