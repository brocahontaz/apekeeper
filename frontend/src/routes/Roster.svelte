<script lang="ts">
  import { onMount } from "svelte";
  import { client, type Character } from "$lib/api";
  import { rosterQuery, type RosterDirection, type RosterSort } from "$lib/roster";
  import { debounce } from "$lib/debounce";
  import RosterTable from "$components/RosterTable.svelte";
  const url = new URLSearchParams(location.search);
  let rows = $state<Character[]>([]);
  let search = $state(url.get("search") ?? "");
  let klass = $state(url.get("class") ?? "");
  let spec = $state(url.get("spec") ?? "");
  let minLevel = $state<number | null>(Number(url.get("minLevel")) || null);
  let minRating = $state<number | null>(Number(url.get("minRating")) || null);
  let stale = $state(url.get("stale") === "true");
  let page = $state(Number(url.get("page")) || 1);
  let sort = $state<RosterSort>((url.get("sort") as RosterSort) || "guildRank");
  let direction = $state<RosterDirection>(
    url.get("direction") === "descending" ? "descending" : "ascending",
  );
  const pageSize = 25;
  let total = $state(0);
  let loading = $state(true);
  let error = $state("");
  let classes = $state<string[]>([]);
  let specs = $state<string[]>([]);
  let pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)));
  async function load() {
    loading = true;
    try {
      const result = await client.roster(
        `?${rosterQuery({
          search,
          class: klass,
          spec,
          minLevel: minLevel ?? undefined,
          minRating: minRating ?? undefined,
          stale,
          page,
          pageSize,
          sort,
          direction,
        })}`,
      );
      rows = result.items;
      total = result.total;
      classes = result.classes;
      specs = result.specs;
    } catch (e) {
      error = e instanceof Error ? e.message : "Roster unavailable";
    } finally {
      loading = false;
    }
  }
  // Typing should not reload per keystroke; one quiet 300ms window schedules a
  // single reload, and teardown cancels anything still pending.
  const searchChanged = debounce(() => {
    page = 1;
    update();
  }, 300);
  onMount(() => {
    load();
    return () => searchChanged.cancel();
  });
  function update() {
    const q = new URLSearchParams();
    if (search) q.set("search", search);
    if (klass) q.set("class", klass);
    if (spec) q.set("spec", spec);
    if (minLevel) q.set("minLevel", String(minLevel));
    if (minRating !== null) q.set("minRating", String(minRating));
    if (stale) q.set("stale", "true");
    if (page > 1) q.set("page", String(page));
    if (sort !== "guildRank") q.set("sort", sort);
    if (direction !== "ascending") q.set("direction", direction);
    history.replaceState({}, "", `/roster?${q}`);
    load();
  }
  function changeSort(column: RosterSort) {
    direction = sort === column && direction === "ascending" ? "descending" : "ascending";
    sort = column;
    page = 1;
    update();
  }
</script>

<h1>Roster</h1>
<div class="toolbar">
  <input
    aria-label="Search roster"
    placeholder="Search ape…"
    bind:value={search}
    oninput={() => searchChanged()}
  /><select
    aria-label="Class"
    bind:value={klass}
    onchange={() => {
      page = 1;
      update();
    }}
    ><option value="">All classes</option>{#each classes as c}<option>{c}</option>{/each}</select
  ><select
    aria-label="Specialization"
    bind:value={spec}
    onchange={() => {
      page = 1;
      update();
    }}
    ><option value="">All specs</option>{#each specs as s}<option>{s}</option>{/each}</select
  ><input
    type="number"
    class="num"
    aria-label="Minimum level"
    min="0"
    bind:value={minLevel}
    onchange={() => {
      page = 1;
      update();
    }}
  /><input
    type="number"
    class="num"
    aria-label="Minimum rating"
    min="0"
    bind:value={minRating}
    onchange={() => {
      page = 1;
      update();
    }}
  /><label
    ><input
      type="checkbox"
      bind:checked={stale}
      onchange={() => {
        page = 1;
        update();
      }}
    /> Stale only</label
  ><button
    onclick={() => {
      search = "";
      klass = "";
      spec = "";
      minLevel = null;
      minRating = null;
      stale = false;
      update();
    }}>Clear</button
  >
</div>
{#if error}<p class="error">{error}</p>{:else if loading}<p class="skeleton">
    Loading roster…
  </p>{:else}<p class="count">{total} apes found · page {page} of {pageCount}</p>
  {#if rows.length}<RosterTable {rows} {sort} {direction} onSort={changeSort} />{:else}<p
      class="empty"
    >
      No apes found.
    </p>{/if}
  <nav class="pagination" aria-label="Roster pages">
    <button
      disabled={page === 1}
      onclick={() => {
        page--;
        update();
      }}>Previous</button
    >
    <span>Page {page} of {pageCount}</span>
    <button
      disabled={page === pageCount}
      onclick={() => {
        page++;
        update();
      }}>Next</button
    >
  </nav>{/if}
