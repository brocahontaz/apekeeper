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
    groupBy = "none",
  }: {
    rows: Character[];
    sort: RosterSort;
    direction: RosterDirection;
    onSort: (column: RosterSort) => void;
    groupBy?: string;
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
  const storageKey = "apekeeper.roster.columns";
  let visible = $state<string[]>(columns.map(([c]) => c));
  if (typeof localStorage !== "undefined") {
    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) ?? "null");
      if (Array.isArray(saved)) {
        const selected = columns.map(([c]) => c).filter((c) => saved.includes(c));
        if (selected.length) visible = selected;
      }
    } catch {}
  }
  const shown = () => columns.filter(([c]) => visible.includes(c));
  function toggle(column: string) {
    if (visible.length === 1 && visible.includes(column)) return;
    visible = visible.includes(column) ? visible.filter((c) => c !== column) : [...visible, column];
    localStorage?.setItem(storageKey, JSON.stringify(visible));
  }
  function group(row: Character) {
    if (groupBy === "class") return row.className || "Unknown class";
    if (groupBy === "role") return String(row.role || "member");
    if (groupBy === "realm") return row.realm || "Unknown realm";
    if (groupBy === "guildRank") return String(row.guildRank ?? "—");
    if (groupBy === "activity") return row.stale ? "Stale" : "Active";
    if (groupBy === "progression") return row.mythicRating > 0 ? "Rated" : "Unrated";
    return "";
  }
  const groupedRows = $derived(
    groupBy === "none"
      ? rows
      : [...rows].sort((a, b) => group(a).localeCompare(group(b)) || a.name.localeCompare(b.name)),
  );
  const characterHref = (name: string) =>
    `/characters/${encodeURIComponent(name)}?roster=${encodeURIComponent(location.search)}`;
</script>

<fieldset class="column-picker">
  <legend>Visible columns</legend>{#each columns as [column, label]}<label
      ><input type="checkbox" checked={visible.includes(column)} onchange={() => toggle(column)} />
      {label}</label
    >{/each}
</fieldset>
<div class="table-wrap">
  <table>
    <thead
      ><tr
        >{#each shown() as [column, label]}<th aria-sort={sort === column ? direction : "none"}
            ><button onclick={() => onSort(column)}>{label}</button></th
          >{/each}</tr
      ></thead
    ><tbody>
      {#each groupedRows as row, index}
        {#if groupBy !== "none" && (index === 0 || group(row) !== group(groupedRows[index - 1]))}<tr
            class="roster-group"><th colspan={shown().length}>{group(row)}</th></tr
          >{/if}
        <tr
          tabindex="0"
          role="link"
          onclick={() => navigate(characterHref(row.name))}
          onkeydown={(e) => e.key === "Enter" && navigate(characterHref(row.name))}
        >
          {#each shown() as [column]}
            <td
              >{#if column === "name"}<strong style={`color:${classColor(row.classId)}`}
                  >{row.name}</strong
                >{:else if column === "level"}{row.level}{:else if column === "itemLevel"}{row.itemLevel.toFixed?.(
                  1,
                ) ?? row.itemLevel}{:else if column === "guildRank"}{row.guildRank ??
                  "—"}{:else if column === "realm"}{row.realm}{:else if column === "classSpec"}{row.className}
                <small>{row.specName}</small>{:else if column === "mythicRating"}{row.mythicRating >
                0
                  ? `${Math.round(row.mythicRating).toLocaleString()} (${row.bestKeyLevel > 0 ? `+${row.bestKeyLevel}` : "—"})`
                  : "—"}{:else}{relativeTime(row.syncedAt)}{#if row.stale}<em class="stale"
                    >stale</em
                  >{/if}{/if}</td
            >
          {/each}
        </tr>
      {/each}
    </tbody>
  </table>
</div>
<div class="roster-cards" aria-label="Roster cards">
  {#each rows as row}<a
      class="roster-card"
      href={characterHref(row.name)}
      onclick={(e) => {
        e.preventDefault();
        navigate(characterHref(row.name));
      }}
      ><strong style={`color:${classColor(row.classId)}`}>{row.name}</strong><span
        >{row.className} · {row.specName}</span
      ><span>Lvl {row.level} · iLvl {row.itemLevel.toFixed?.(1) ?? row.itemLevel}</span><span
        >{row.realm} · {row.mythicRating > 0
          ? `M+ ${Math.round(row.mythicRating)}`
          : "Unrated"}</span
      ></a
    >{/each}
</div>
