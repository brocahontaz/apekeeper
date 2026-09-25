<script lang="ts">
  import { classColor } from "$lib/theme";
  import { relativeTime } from "$lib/format";
  import { navigate } from "$lib/router";
  import type { Character } from "$lib/api";
  let { rows = [] }: { rows: Character[] } = $props();
  let sort = $state<"name" | "level" | "itemLevel">("name");
  let sorted = $derived(
    [...rows].sort((a, b) =>
      sort === "name" ? a.name.localeCompare(b.name) : (b[sort] as number) - (a[sort] as number),
    ),
  );
</script>

<div class="table-wrap">
  <table>
    <thead
      ><tr
        >{#each [["name", "Keeper"], ["level", "Lvl"], ["itemLevel", "iLvl"]] as entry}<th
            ><button onclick={() => (sort = entry[0] as typeof sort)}>{entry[1]}</button></th
          >{/each}<th>Realm</th><th>Class / spec</th><th>M+</th><th>Last record</th></tr
      ></thead
    ><tbody
      >{#each sorted as row}<tr
          tabindex="0"
          role="link"
          onclick={() => navigate(`/characters/${row.id}`)}
          onkeydown={(e) => e.key === "Enter" && navigate(`/characters/${row.id}`)}
          ><td><strong style={`color:${classColor(row.classId)}`}>{row.name}</strong></td><td
            >{row.level}</td
          ><td>{row.itemLevel.toFixed?.(1) ?? row.itemLevel}</td><td>{row.realm}</td><td
            >{row.className} <small>{row.specName}</small></td
          ><td
            >{row.mythicRating > 0
              ? `${Math.round(row.mythicRating).toLocaleString()} (${row.bestKeyLevel > 0 ? `+${row.bestKeyLevel}` : "—"})`
              : "—"}</td
          ><td
            >{relativeTime(row.syncedAt)}
            {#if row.stale}<em class="stale">stale</em>{/if}</td
          ></tr
        >{/each}</tbody
    >
  </table>
</div>
