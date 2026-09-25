<script lang="ts">
  import { onMount } from "svelte";
  import { client, type Character } from "$lib/api";
  import { rosterQuery, type RosterDirection, type RosterSort } from "$lib/roster";
  import RosterTable from "$components/RosterTable.svelte";
  let rows = $state<Character[]>([]);
  let search = $state(new URLSearchParams(location.search).get("search") ?? "");
  let klass = $state(new URLSearchParams(location.search).get("class") ?? "");
  let stale = $state(new URLSearchParams(location.search).get("stale") === "true");
  let page = $state(Number(new URLSearchParams(location.search).get("page")) || 1);
  let sort = $state<RosterSort>(
    (new URLSearchParams(location.search).get("sort") as RosterSort) || "name",
  );
  let direction = $state<RosterDirection>(
    new URLSearchParams(location.search).get("direction") === "descending"
      ? "descending"
      : "ascending",
  );
  const pageSize = 25;
  let total = $state(0);
  let loading = $state(true);
  let error = $state("");
  let classes = $state<string[]>([]);
  let pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)));
  async function load() {
    loading = true;
    try {
      const result = await client.roster(
        `?${rosterQuery({ search, class: klass, stale, page, pageSize, sort, direction })}`,
      );
      rows = result.items;
      total = result.total;
      classes = result.classes;
    } catch (e) {
      error = e instanceof Error ? e.message : "Roster unavailable";
    } finally {
      loading = false;
    }
  }
  onMount(load);
  function update() {
    const q = new URLSearchParams();
    if (search) q.set("search", search);
    if (klass) q.set("class", klass);
    if (stale) q.set("stale", "true");
    if (page > 1) q.set("page", String(page));
    if (sort !== "name") q.set("sort", sort);
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
    oninput={() => {
      page = 1;
      update();
    }}
  /><select
    aria-label="Class"
    bind:value={klass}
    onchange={() => {
      page = 1;
      update();
    }}
    ><option value="">All classes</option>{#each classes as c}<option>{c}</option>{/each}</select
  ><label
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
