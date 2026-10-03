<script lang="ts">
  import { client, ApiError, type SyncProgress, type SyncRun } from "$lib/api";
  import { onDestroy } from "svelte";
  import { relativeTime } from "$lib/format";
  import { currentUser, effectiveRole, selectedGuildRole } from "$lib/stores";
  import {
    canTriggerSync,
    canViewRuns as canViewRunsFor,
    canViewProgress as canViewProgressFor,
    failureDetails,
    notifyStatusChip,
    phaseLabel,
    progressActive,
  } from "$lib/sync";
  let runs = $state<SyncRun[]>([]);
  let error = $state("");
  let busy = $state(false);
  let loaded = $state(false);
  let loading = $state(false);
  let generation = 0;
  let progress = $state<SyncProgress | null>(null);
  let polling = $state(false);
  let completionAnnouncement = $state("");
  let role = $derived(effectiveRole($currentUser, $selectedGuildRole));
  let canTrigger = $derived(canTriggerSync(role));
  let canViewRuns = $derived(canViewRunsFor(role));
  let canViewProgress = $derived(canViewProgressFor(role));
  $effect(() => {
    if (canViewRuns && !loaded) load();
  });
  // Live progress polls while a run is in flight and stops on its own once
  // the snapshot goes terminal, so a manual run transitions without a
  // refresh and the finished row takes over.
  $effect(() => {
    if (!polling || !canViewProgress) return;
    poll();
    const timer = setInterval(poll, 1500);
    return () => clearInterval(timer);
  });
  async function load() {
    const request = ++generation;
    loading = true;
    error = "";
    try {
      const result = await client.runs();
      if (request !== generation) return;
      runs = result;
      loaded = true;
      if (runs[0]?.status === "running") startPolling();
    } catch (e) {
      if (request === generation)
        error = e instanceof Error ? e.message : "Sync history unavailable";
    } finally {
      if (request === generation) loading = false;
    }
  }
  function startPolling() {
    if (canViewProgress) polling = true;
  }
  function stopPolling() {
    polling = false;
    progress = null;
  }
  async function poll() {
    const request = generation;
    try {
      const p = await client.progress();
      if (request !== generation) return;
      progress = p;
      if (!progressActive(p)) {
        completionAnnouncement =
          p.status === "success"
            ? "Sync completed successfully."
            : `Sync finished with status: ${p.status}.`;
        stopPolling();
        await load();
      }
    } catch {
      // A failed poll ends the live view quietly; the history table still
      // holds the last recorded state.
      stopPolling();
    }
  }
  onDestroy(() => generation++);
  async function trigger() {
    if (!confirm("Start a full guild sync?")) return;
    busy = true;
    error = "";
    try {
      await client.sync();
      await load();
      startPolling();
    } catch (e) {
      error = triggerError(e);
    } finally {
      busy = false;
    }
  }
  async function dryRun() {
    busy = true;
    error = "";
    try {
      await client.dryRun();
      await load();
      startPolling();
    } catch (e) {
      error = triggerError(e);
    } finally {
      busy = false;
    }
  }
  async function cancel() {
    busy = true;
    error = "";
    try {
      await client.cancel();
      startPolling();
    } catch (e) {
      error = triggerError(e);
    } finally {
      busy = false;
    }
  }
  async function retry(run: SyncRun) {
    busy = true;
    error = "";
    try {
      await client.retry(run.id);
      await load();
      startPolling();
    } catch (e) {
      error = triggerError(e);
    } finally {
      busy = false;
    }
  }
  function triggerError(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 409) return "A sync is already running";
      if (e.status === 503) return "Sync unavailable";
      return e.message;
    }
    return "Sync could not start";
  }
</script>

<h1>Keeper Controls</h1>
<section class="controls">
  <h2>Guild expedition sync</h2>
  {#if canTrigger}<button class="gold" disabled={busy} onclick={trigger}
      >{busy ? "Dispatching…" : "Trigger full sync"}</button
    ><button disabled={busy} onclick={dryRun}>Dry run</button
    >{#if progress && progressActive(progress)}<button disabled={busy} onclick={cancel}
        >Cancel sync</button
      >{/if}
    >{:else}<span title="Only Guild Masters can trigger a full sync"
      >Trigger restricted to Guild Master</span
    >{/if}
  <p>Scheduled runs keep the ledger fresh nightly at 03:00 UTC.</p>
</section>
{#if error}<section class="error-state" role="alert" aria-live="assertive">
    <p class="error">{error}</p>
    <button onclick={load}>Retry sync history</button>
  </section>{/if}
{#if completionAnnouncement}<p
    class="sync-status"
    role="status"
    aria-live="polite"
    aria-atomic="true"
  >
    {completionAnnouncement}
  </p>{/if}
{#if loading && !runs.length}<p class="skeleton" role="status" aria-live="polite">
    Loading sync history…
  </p>{/if}
{#if canViewProgress && progress && progressActive(progress)}
  <section aria-live="polite">
    <h2>Expedition in progress</h2>
    <p class="count">
      <span class="chip">{progress.status}</span>
      <span class="chip">{phaseLabel(progress.phase)}</span>
      {progress.updated} updated · {progress.failed} failed · {progress.total} apes
    </p>
  </section>
{/if}
{#if role === "member"}<section>
    <h2>Sync run history</h2>
    <p class="restricted">
      Run history is sealed to officers and above. Ask an officer for the latest expedition report.
    </p>
  </section>{:else if canViewRuns}<section>
    <h2>Sync run history</h2>
    {#if loading}<p class="refreshing" role="status">Refreshing sync history…</p>{/if}
    <div class="table-wrap">
      <table>
        <thead
          ><tr>
            <th>Started</th><th>Trigger</th><th>Status</th><th>Updated</th><th>Failed</th><th
              >Total</th
            >{#if canTrigger}<th>Actions</th>{/if}
          </tr></thead
        ><tbody
          >{#each runs as run}{@const notify = notifyStatusChip(run.notifyStatus)}<tr
              ><td>{relativeTime(run.startedAt)}</td><td>{run.trigger}</td><td
                ><span class="chip">{run.status}</span>{#if notify}<span class="chip {notify.kind}"
                    >{notify.label}</span
                  >{/if}{#if run.failed > 0 && run.errorSummary}<small class="error-line"
                    >{run.errorSummary}</small
                  >{/if}</td
              ><td>{run.updated}</td><td>{run.failed}</td><td>{run.total}</td>{#if canTrigger}<td
                  >{#if run.failed > 0}<button disabled={busy} onclick={() => retry(run)}
                      >Retry failed</button
                    >{/if}</td
                >{/if}</tr
            >{#if run.failed > 0}<tr class="detail-row"
                ><td colspan={canTrigger ? 7 : 6}>
                  <details class="run-failures">
                    <summary>Failure detail · {run.failed} apes</summary>
                    <ul class="failure-detail">
                      {#each failureDetails(run) as f}<li>
                          <strong>{f.name}</strong>
                          {f.error}
                        </li>{/each}
                    </ul>
                  </details>
                </td></tr
              >{/if}{:else}<tr><td colspan="6" aria-live="polite">No sync runs recorded.</td></tr
            >{/each}</tbody
        >
      </table>
    </div>
  </section>{/if}
<aside class="guidance">
  <h2>Battle.net credentials</h2>
  <p>
    Set client credentials before your first sync. The keeper's setup guide lives at
    <code>docs/blizzard-setup.md</code> in the repo.
  </p>
</aside>
