<script lang="ts">
  import type { Snapshot } from "$lib/api";
  let { snapshots }: { snapshots: Snapshot[] } = $props();
  const valid = (value: unknown): value is number =>
    typeof value === "number" && Number.isFinite(value);
  let points = $derived(
    snapshots.filter(
      (s) =>
        !!s &&
        valid(s.itemLevel) &&
        valid(s.mythicRating) &&
        typeof s.capturedAt === "string" &&
        !Number.isNaN(Date.parse(s.capturedAt)),
    ),
  );
  function chart(metric: "itemLevel" | "mythicRating") {
    const values = points.map((p) => p[metric]);
    const low = Math.min(...values),
      high = Math.max(...values),
      span = high - low || 1;
    return points
      .map(
        (p, i) =>
          `${points.length === 1 ? 50 : (i / (points.length - 1)) * 100},${100 - ((p[metric] - low) / span) * 100}`,
      )
      .join(" ");
  }
  function summary(metric: "itemLevel" | "mythicRating", label: string) {
    if (!points.length) return `No valid ${label} history is available.`;
    const first = points[0][metric],
      last = points[points.length - 1][metric];
    return `${label} changed from ${Math.round(first).toLocaleString()} to ${Math.round(last).toLocaleString()} across ${points.length} snapshot${points.length === 1 ? "" : "s"}.`;
  }
</script>

{#if snapshots.length === 0}<p>No snapshots in this date range.</p>
{:else if points.length === 0}<p class="error">History data is incomplete and cannot be charted.</p>
{:else}
  <div class="history-charts">
    <figure>
      <figcaption>Item level over time</figcaption>
      <svg
        viewBox="0 0 100 100"
        role="img"
        aria-label={summary("itemLevel", "Item level")}
        preserveAspectRatio="none"
        ><polyline
          points={chart("itemLevel")}
          fill="none"
          stroke="currentColor"
          stroke-width="3"
          vector-effect="non-scaling-stroke"
        /></svg
      >
      <p>{summary("itemLevel", "Item level")}</p>
    </figure>
    <figure>
      <figcaption>Mythic+ rating over time</figcaption>
      <svg
        viewBox="0 0 100 100"
        role="img"
        aria-label={summary("mythicRating", "Mythic+ rating")}
        preserveAspectRatio="none"
        ><polyline
          points={chart("mythicRating")}
          fill="none"
          stroke="currentColor"
          stroke-width="3"
          vector-effect="non-scaling-stroke"
        /></svg
      >
      <p>{summary("mythicRating", "Mythic+ rating")}</p>
    </figure>
  </div>
{/if}
