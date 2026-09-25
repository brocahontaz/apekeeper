<script lang="ts">
  import { classColor } from "$lib/theme";
  let { classes = [] }: { classes: { classId: number; className: string; count: number }[] } =
    $props();
  let max = $derived(Math.max(1, ...classes.map((c) => c.count)));
</script>

<div class="distribution">
  {#each classes as c}<div class="dist-row">
      <span>{c.className}</span>
      <div class="bar-track">
        <i style={`width:${(c.count / max) * 100}%;background:${classColor(c.classId)}`}></i>
      </div>
      <b>{c.count}</b>
    </div>{/each}
</div>
