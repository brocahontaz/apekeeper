<script lang="ts">
  import { classColor } from "$lib/theme";
  import { relativeTime } from "$lib/format";
  import { navigate } from "$lib/router";
  import type { Character } from "$lib/api";
  import type { RosterDirection, RosterSort } from "$lib/roster";
  let {
    rows = [],
    sort,
    direction,
    onSort,
  }: {
    rows: Character[];
    sort: RosterSort;
    direction: RosterDirection;
    onSort: (column: RosterSort) => void;
  } = $props();
  const columns: [RosterSort, string][] = [
    ["name", "Ape"],
    ["level", "Lvl"],
    ["itemLevel", "iLvl"],
    ["guildRank", "Rank"],
    ["realm", "Realm"],
    ["classSpec", "Class / spec"],
    ["mythicRating", "M+"],
    ["syncedAt", "Last record"],
  ];
</script>

<div class="table-wrap">
  <table>
    <thead
      ><tr
        >{#each columns as [column, label]}<th aria-sort={sort === column ? direction : "none"}
            ><button onclick={() => onSort(column)}>{label}</button></th
          >{/each}</tr
      ></thead
    ><tbody
      >{#each rows as row}<tr
          tabindex="0"
          role="link"
          onclick={() => navigate(`/characters/${encodeURIComponent(row.name)}`)}
          onkeydown={(e) =>
            e.key === "Enter" && navigate(`/characters/${encodeURIComponent(row.name)}`)}
          ><td><strong style={`color:${classColor(row.classId)}`}>{row.name}</strong></td><td
            >{row.level}</td
          ><td>{row.itemLevel.toFixed?.(1) ?? row.itemLevel}</td><td>{row.guildRank ?? "—"}</td><td
            >{row.realm}</td
          ><td>{row.className} <small>{row.specName}</small></td><td
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
