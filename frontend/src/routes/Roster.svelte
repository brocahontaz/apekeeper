<script lang="ts">
  import { onMount } from "svelte";
  import { client, type Character } from "$lib/api";
  import {
    rosterQuery,
    rosterExportQuery,
    type RosterDirection,
    type RosterSort,
  } from "$lib/roster";
  import { debounce } from "$lib/debounce";
  import RosterTable from "$components/RosterTable.svelte";
  const read = () => new URLSearchParams(location.search);
  const u = read();
  let rows = $state<Character[]>([]),
    total = $state(0),
    loading = $state(true),
    error = $state("");
  let search = $state(u.get("search") ?? ""),
    klass = $state(u.get("class") ?? ""),
    spec = $state(u.get("spec") ?? "");
  let minLevel = $state<number | null>(Number(u.get("minLevel")) || null),
    minRating = $state<number | null>(Number(u.get("minRating")) || null);
  let stale = $state(u.get("stale") === "true"),
    page = $state(Number(u.get("page")) || 1);
  let sort = $state<RosterSort>((u.get("sort") as RosterSort) || "guildRank"),
    direction = $state<RosterDirection>(
      u.get("direction") === "descending" ? "descending" : "ascending",
    );
  let mythicSeason = $state(u.get("mythicSeason") ?? ""),
    raidTier = $state(u.get("raidTier") ?? ""),
    raidDifficulty = $state(u.get("raidDifficulty") ?? "");
  let minRaidProgress = $state<number | null>(Number(u.get("minRaidProgress")) || null),
    activityAge = $state<number | null>(Number(u.get("activityAge")) || null);
  let officerStatus = $state(u.get("officerStatus") ?? ""),
    officerTag = $state(u.get("officerTag") ?? ""),
    role = $state(u.get("role") ?? ""),
    groupBy = $state(u.get("groupBy") ?? "none");
  let classes = $state<string[]>([]),
    specs = $state<string[]>([]);
  const pageSize = 25;
  let generation = 0;
  let pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)));
  const filters = () => ({
    search,
    class: klass,
    spec,
    minLevel: minLevel ?? undefined,
    minRating: minRating ?? undefined,
    stale,
    mythicSeason,
    raidTier,
    raidDifficulty,
    minRaidProgress: minRaidProgress ?? undefined,
    activityAge: activityAge ?? undefined,
    officerStatus,
    officerTag,
    role,
  });
  async function load() {
    const request = ++generation;
    loading = true;
    error = "";
    try {
      const r = await client.roster(
        `?${rosterQuery({ ...filters(), page, pageSize, sort, direction })}`,
      );
      if (request !== generation) return;
      rows = r.items;
      total = r.total;
      classes = r.classes;
      specs = r.specs;
    } catch (e) {
      if (request === generation) error = e instanceof Error ? e.message : "Roster unavailable";
    } finally {
      if (request === generation) loading = false;
    }
  }
  function update() {
    const q = new URLSearchParams(rosterQuery({ ...filters(), page, pageSize, sort, direction }));
    if (groupBy !== "none") q.set("groupBy", groupBy);
    history.pushState({}, "", `/roster?${q}`);
    load();
  }
  const searchChanged = debounce(() => {
    page = 1;
    update();
  }, 300);
  function changeFilter() {
    page = 1;
    update();
  }
  function changeSort(column: RosterSort) {
    direction = sort === column && direction === "ascending" ? "descending" : "ascending";
    sort = column;
    page = 1;
    update();
  }
  function clearFilters() {
    search = "";
    klass = "";
    spec = "";
    minLevel = null;
    minRating = null;
    stale = false;
    mythicSeason = "";
    raidTier = "";
    raidDifficulty = "";
    minRaidProgress = null;
    activityAge = null;
    officerStatus = "";
    officerTag = "";
    role = "";
    changeFilter();
  }
  onMount(() => {
    load();
    const restore = () => {
      const q = read();
      search = q.get("search") ?? "";
      klass = q.get("class") ?? "";
      spec = q.get("spec") ?? "";
      minLevel = Number(q.get("minLevel")) || null;
      minRating = Number(q.get("minRating")) || null;
      stale = q.get("stale") === "true";
      mythicSeason = q.get("mythicSeason") ?? "";
      raidTier = q.get("raidTier") ?? "";
      raidDifficulty = q.get("raidDifficulty") ?? "";
      minRaidProgress = Number(q.get("minRaidProgress")) || null;
      activityAge = Number(q.get("activityAge")) || null;
      officerStatus = q.get("officerStatus") ?? "";
      officerTag = q.get("officerTag") ?? "";
      role = q.get("role") ?? "";
      groupBy = q.get("groupBy") ?? "none";
      page = Number(q.get("page")) || 1;
      sort = (q.get("sort") as RosterSort) || "guildRank";
      direction = q.get("direction") === "descending" ? "descending" : "ascending";
      load();
    };
    addEventListener("popstate", restore);
    return () => {
      searchChanged.cancel();
      generation++;
      removeEventListener("popstate", restore);
    };
  });
</script>

<h1>Roster</h1>
<div class="toolbar">
  <input
    aria-label="Search roster"
    placeholder="Search ape…"
    bind:value={search}
    oninput={() => searchChanged()}
  />
  <select aria-label="Class" bind:value={klass} onchange={changeFilter}
    ><option value="">All classes</option>{#each classes as c}<option>{c}</option>{/each}</select
  >
  <select aria-label="Account role" bind:value={role} onchange={changeFilter}
    ><option value="">All account roles</option
    >{#each ["member", "officer", "admin", "superadmin"] as accountRole}<option value={accountRole}
        >{accountRole}</option
      >{/each}</select
  >
  <select aria-label="Specialization" bind:value={spec} onchange={changeFilter}
    ><option value="">All specs</option>{#each specs as s}<option>{s}</option>{/each}</select
  >
  <input
    type="number"
    class="num"
    aria-label="Minimum level"
    min="0"
    bind:value={minLevel}
    onchange={changeFilter}
  />
  <input
    type="number"
    class="num"
    aria-label="Minimum rating"
    min="0"
    bind:value={minRating}
    onchange={changeFilter}
  />
  <label><input type="checkbox" bind:checked={stale} onchange={changeFilter} /> Stale only</label>
  <input
    class="num"
    aria-label="Mythic+ season"
    placeholder="M+ season"
    bind:value={mythicSeason}
    onchange={changeFilter}
  />
  <input
    class="num"
    aria-label="Raid tier"
    placeholder="Raid tier"
    bind:value={raidTier}
    onchange={changeFilter}
  />
  <input
    class="num"
    aria-label="Raid difficulty"
    placeholder="Difficulty"
    bind:value={raidDifficulty}
    onchange={changeFilter}
  />
  <input
    type="number"
    class="num"
    aria-label="Minimum raid progress"
    min="0"
    bind:value={minRaidProgress}
    onchange={changeFilter}
  />
  <input
    type="number"
    class="num"
    aria-label="Activity age in days"
    min="0"
    bind:value={activityAge}
    onchange={changeFilter}
  />
  <select aria-label="Officer status" bind:value={officerStatus} onchange={changeFilter}
    ><option value="">All officer statuses</option
    >{#each ["applicant", "trial", "active", "inactive", "retired"] as status}<option value={status}
        >{status}</option
      >{/each}</select
  >
  <input
    class="num"
    aria-label="Officer tag"
    placeholder="Officer tag"
    bind:value={officerTag}
    onchange={changeFilter}
  />
  <select aria-label="Group roster by" bind:value={groupBy} onchange={update}
    ><option value="none">No grouping</option
    >{#each ["class", "role", "realm", "guildRank", "activity", "progression"] as mode}<option
        value={mode}>{mode}</option
      >{/each}</select
  >
  <button onclick={clearFilters}>Clear</button>
  <a
    class="export"
    href={`/api/roster/export?${rosterExportQuery({ ...filters(), sort, direction })}`}
    download>Export CSV</a
  >
</div>
{#if error}<section class="error-state" role="alert" aria-live="assertive">
    <p class="error">{error}</p>
    <button onclick={load}>Retry roster request</button>
  </section>{/if}
{#if loading && !rows.length && !error}<p class="skeleton" role="status">
    Loading roster…
  </p>{:else if rows.length || !error}
  {#if loading}<p class="refreshing" role="status" aria-live="polite">Refreshing roster…</p>{/if}
  <p class="count" aria-live="polite">
    Showing {rows.length} of {total} matching apes · page {page} of {pageCount}
  </p>
  {#if rows.length}<RosterTable {rows} {sort} {direction} {groupBy} onSort={changeSort} />{:else}<p
      class="empty"
      role="status"
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
    ><span>Page {page} of {pageCount}</span><button
      disabled={page === pageCount}
      onclick={() => {
        page++;
        update();
      }}>Next</button
    >
  </nav>
{/if}
