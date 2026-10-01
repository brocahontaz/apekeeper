<script lang="ts">
  import { theme, toggleTheme } from "$lib/colorScheme";
  let dark = $derived($theme === "dark");
</script>

<button
  onclick={toggleTheme}
  aria-pressed={dark}
  aria-label="Toggle dark mode"
  title="Toggle dark mode">{dark ? "☀" : "☾"}</button
>
