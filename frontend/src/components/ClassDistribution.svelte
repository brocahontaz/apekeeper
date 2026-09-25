<script lang="ts">
  import { classColor } from "$lib/theme";
  import { sortClasses } from "$lib/distribution";
  let {
    classes = [],
  }: {
    classes: {
      classId: number;
      className: string;
      count: number;
      specs: { name: string; count: number }[];
    }[];
  } = $props();
  let sorted = $derived(sortClasses(classes));
  let max = $derived(Math.max(1, ...sorted.map((c) => c.count)));
</script>

<div class="distribution">
  {#each sorted as c (c.className)}<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div class="dist-row" tabindex="0">
      <span>{c.className}</span>
      <div class="bar-track">
        <i style={`width:${(c.count / max) * 100}%;background:${classColor(c.classId)}`}></i>
      </div>
      <b>{c.count}</b>
      {#if c.specs.length}<span class="dist-tip" role="tooltip"
          >{c.specs.map((s) => `${s.name} ${s.count}`).join(" · ")}</span
        >{/if}
    </div>{/each}
</div>
