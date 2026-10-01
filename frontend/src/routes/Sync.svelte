<script lang="ts">
  import { client, ApiError, type SyncRun } from "$lib/api";
  import { relativeTime } from "$lib/format";
  import { currentUser } from "$lib/stores";
  import {
    canTriggerSync,
    canViewRuns as canViewRunsFor,
    failureDetails,
    notifyStatusChip,
  } from "$lib/sync";
  let runs = $state<SyncRun[]>([]);
  let error = $state("");
  let busy = $state(false);
  let loaded = $state(false);
  let role = $derived($currentUser?.appRole);
  let canTrigger = $derived(canTriggerSync(role));
  let canViewRuns = $derived(canViewRunsFor(role));
  $effect(() => {
    if (canViewRuns && !loaded) load();
  });
  async function load() {
    try {
      runs = await client.runs();
      loaded = true;
    } catch (e) {
      error = e instanceof Error ? e.message : "Sync history unavailable";
    }
  }
  async function trigger() {
    if (!confirm("Start a full guild sync?")) return;
    busy = true;
    try {
      await client.sync();
      await load();
    } catch (e) {
      error = e instanceof ApiError ? e.message : "Sync could not start";
    } finally {
      busy = false;
    }
  }
</script>

<h1>Keeper Controls</h1>
<section class="controls">
  <h2>Guild expedition sync</h2>
  {#if canTrigger}<button class="gold" disabled={busy} onclick={trigger}
      >{busy ? "Dispatching…" : "Trigger full sync"}</button
    >{:else}<span title="Only Guild Masters can trigger a full sync"
      >Trigger restricted to Guild Master</span
    >{/if}
  <p>Scheduled runs keep the ledger fresh nightly at 03:00 UTC.</p>
</section>
{#if error}<p class="error" role="status">{error}</p>{/if}
{#if role === "member"}<section>
    <h2>Sync run history</h2>
    <p class="restricted">
      Run history is sealed to officers and above. Ask an officer for the latest expedition report.
    </p>
  </section>{:else if canViewRuns}<section>
    <h2>Sync run history</h2>
    <div class="table-wrap">
      <table>
        <thead
          ><tr>
            <th>Started</th><th>Trigger</th><th>Status</th><th>Updated</th><th>Failed</th><th
              >Total</th
            >
          </tr></thead
        ><tbody
          >{#each runs as run}{@const notify = notifyStatusChip(run.notifyStatus)}<tr
              ><td>{relativeTime(run.startedAt)}</td><td>{run.trigger}</td><td
                ><span class="chip">{run.status}</span>{#if notify}<span class="chip {notify.kind}"
                    >{notify.label}</span
                  >{/if}{#if run.failed > 0 && run.errorSummary}<small class="error-line"
                    >{run.errorSummary}</small
                  >{/if}</td
              ><td>{run.updated}</td><td>{run.failed}</td><td>{run.total}</td></tr
            >{#if run.failed > 0}<tr class="detail-row"
                ><td colspan="6">
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
