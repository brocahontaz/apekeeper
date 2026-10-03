<script lang="ts">
  import { client, type OfficerActivity, type OfficerQueueItem } from "$lib/api";
  import { currentUser, effectiveRole, selectedGuildRole } from "$lib/stores";
  import { onDestroy } from "svelte";
  let items = $state<OfficerQueueItem[]>([]),
    activity = $state<OfficerActivity[]>([]),
    error = $state(""),
    success = $state(""),
    loading = $state(true),
    reason = $state(""),
    selected = $state<string[]>([]),
    tags = $state(""),
    note = $state(""),
    lifecycleStatus = $state(""),
    characterTags = $state(""),
    characterLoading = $state(false);
  let alive = true;
  let loadGeneration = 0;
  let characterGeneration = 0;
  let mutationGeneration = 0;
  let mutationVersion = 0;
  let selectionGeneration = 0;
  onDestroy(() => {
    alive = false;
    loadGeneration++;
    characterGeneration++;
    mutationGeneration++;
    mutationVersion++;
  });
  let allowed = $derived(
    !!$currentUser && effectiveRole($currentUser, $selectedGuildRole) !== "member",
  );
  $effect(() => {
    if (allowed) load();
    else loading = false;
  });
  $effect(() => {
    const id = selected.length === 1 ? selectedCharacterIDs[0] : undefined;
    if (!id) return;
    void loadCharacter(id);
  });
  async function load(clearSelection = true) {
    const generation = ++loadGeneration;
    const selection = selectionGeneration;
    const mutation = mutationVersion;
    loading = true;
    error = "";
    try {
      const [q, a] = await Promise.all([client.officerQueue(reason), client.officerActivity()]);
      if (!alive || generation !== loadGeneration || mutation !== mutationVersion) return;
      items = q.items;
      activity = a;
      if (clearSelection && selection === selectionGeneration) selected = [];
    } catch (e) {
      if (!alive || generation !== loadGeneration || mutation !== mutationVersion) return;
      error = e instanceof Error ? e.message : "Officer workflow unavailable";
    } finally {
      if (alive && generation === loadGeneration && mutation === mutationVersion) loading = false;
    }
  }
  let selectedItems = $derived(items.filter((item) => selected.includes(itemID(item))));
  let selectedCharacterIDs = $derived(
    selectedItems.flatMap((item) => (item.characterId ? [item.characterId] : [])),
  );
  let selectedSyncRunIDs = $derived(
    selectedItems.flatMap((item) => (item.syncRunId ? [item.syncRunId] : [])),
  );
  function itemID(item: OfficerQueueItem) {
    return item.characterId ? `character:${item.characterId}` : `sync-run:${item.syncRunId}`;
  }
  function toggle(id: string) {
    selectionGeneration++;
    characterGeneration++;
    error = "";
    characterLoading = false;
    note = "";
    lifecycleStatus = "";
    characterTags = "";
    selected = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id];
  }
  function parsedTags() {
    return tags
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);
  }
  function parsedCharacterTags() {
    return characterTags
      .split(",")
      .map((x) => x.trim())
      .filter(Boolean);
  }
  async function loadCharacter(id: number) {
    const generation = ++characterGeneration;
    characterLoading = true;
    error = "";
    try {
      const character = await client.officerCharacter(id);
      if (
        !alive ||
        generation !== characterGeneration ||
        selected.length !== 1 ||
        selectedCharacterIDs[0] !== id
      )
        return;
      note = character.note;
      lifecycleStatus = character.lifecycleStatus;
      characterTags = character.tags.join(", ");
    } catch (e) {
      if (
        alive &&
        generation === characterGeneration &&
        selected.length === 1 &&
        selectedCharacterIDs[0] === id
      )
        error = e instanceof Error ? e.message : "Officer details could not be loaded";
    } finally {
      if (alive && generation === characterGeneration) characterLoading = false;
    }
  }
  async function apply(add: boolean) {
    if (
      !selected.length ||
      !confirm(`${add ? "Add" : "Remove"} tags for ${selected.length} selected characters?`)
    )
      return;
    const generation = ++mutationGeneration;
    const selection = selectionGeneration;
    try {
      const result = await client.bulkTags(
        selectedCharacterIDs,
        add ? parsedTags() : [],
        add ? [] : parsedTags(),
      );
      if (!alive || generation !== mutationGeneration) return;
      mutationVersion++;
      success = `Updated ${result.updated} characters.`;
      await load(selection === selectionGeneration);
    } catch (e) {
      if (alive && generation === mutationGeneration)
        error = e instanceof Error ? e.message : "Update failed";
    }
  }
  async function complete() {
    if (!selected.length || !confirm(`Mark ${selected.length} selected characters reviewed?`))
      return;
    const generation = ++mutationGeneration;
    const selection = selectionGeneration;
    try {
      const result = await client.completeReview(selectedCharacterIDs, selectedSyncRunIDs);
      if (!alive || generation !== mutationGeneration) return;
      mutationVersion++;
      success = `Completed ${result.completed} reviews.`;
      await load(selection === selectionGeneration);
    } catch (e) {
      if (alive && generation === mutationGeneration)
        error = e instanceof Error ? e.message : "Review failed";
    }
  }
  async function saveCharacter() {
    if (selected.length !== 1 || selectedCharacterIDs.length !== 1) return;
    const generation = ++mutationGeneration;
    const selection = selectionGeneration;
    try {
      await client.updateOfficerCharacter(
        selectedCharacterIDs[0],
        note,
        lifecycleStatus,
        parsedCharacterTags(),
      );
      if (!alive || generation !== mutationGeneration) return;
      mutationVersion++;
      success = "Officer details saved.";
      await load(selection === selectionGeneration);
    } catch (e) {
      if (alive && generation === mutationGeneration)
        error = e instanceof Error ? e.message : "Officer details could not be saved";
    }
  }
</script>

<h1>Officer workflow</h1>
{#if !allowed}<p class="restricted">Officer access is required.</p>{:else}
  <div class="controls">
    <label
      >Queue filter <select bind:value={reason} onchange={() => load()}
        ><option value="">All reasons</option><option value="stale">Stale</option><option
          value="recent_sync_failure">Recent sync failures</option
        ><option value="missing_progression">Missing progression</option><option
          value="unreviewed_change">Unreviewed changes</option
        ></select
      ></label
    ><button onclick={() => load()}>Refresh</button>
  </div>
  {#if error}<p class="error" role="alert">{error}</p>{/if}{#if success}<p role="status">
      {success}
    </p>{/if}
  {#if loading}<p aria-live="polite">Loading officer queue…</p>{:else if items.length === 0}<p
      aria-live="polite"
    >
      No characters need review.
    </p>{:else}
    <div class="controls">
      <label>Tags (comma separated) <input bind:value={tags} maxlength="659" /></label><button
        onclick={() => apply(true)}
        disabled={!selectedCharacterIDs.length}>Add tags</button
      ><button onclick={() => apply(false)} disabled={!selectedCharacterIDs.length}
        >Remove tags</button
      ><button class="gold" onclick={complete} disabled={!selected.length}>Mark reviewed</button>
    </div>
    {#if selected.length === 1 && selectedCharacterIDs.length === 1}
      <fieldset>
        <legend>Selected character details</legend>
        {#if characterLoading}<p aria-live="polite">Loading character details…</p>{/if}
        <label
          >Lifecycle status <select bind:value={lifecycleStatus}
            ><option value="">Not set</option><option value="applicant">Applicant</option><option
              value="trial">Trial</option
            ><option value="active">Active</option><option value="inactive">Inactive</option><option
              value="retired">Retired</option
            ></select
          ></label
        >
        <label>Officer note <textarea bind:value={note} maxlength="2000"></textarea></label>
        <label>Character tags <input bind:value={characterTags} maxlength="659" /></label>
        <button onclick={saveCharacter}>Save details</button>
      </fieldset>
    {/if}
    <table>
      <thead><tr><th>Select</th><th>Target</th><th>Reason</th><th>Tags</th></tr></thead><tbody
        >{#each items as item}<tr
            ><td
              >{#if item.characterId || item.syncRunId}<input
                  aria-label={`Select ${item.name}`}
                  type="checkbox"
                  checked={selected.includes(itemID(item))}
                  onchange={() => toggle(itemID(item))}
                />{/if}</td
            ><td>{item.name}</td><td>{item.reason.replaceAll("_", " ")}</td><td
              >{item.tags.join(", ")}</td
            ></tr
          >{/each}</tbody
      >
    </table>
  {/if}
  <section>
    <h2>Recent officer activity</h2>
    {#if activity.length === 0}<p>No officer activity recorded.</p>{:else}<ul>
        {#each activity as event}<li>
            {event.actor} · {event.action} · target #{event.targetId ?? "—"} · {new Date(
              event.createdAt,
            ).toLocaleString()}{#if Object.keys(event.changes).length}
              · {JSON.stringify(event.changes)}{/if}
          </li>{/each}
      </ul>{/if}
  </section>
{/if}
