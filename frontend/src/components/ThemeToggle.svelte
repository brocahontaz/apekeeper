<script lang="ts">
  import { theme, toggleTheme } from "$lib/colorScheme";
  let dark = $derived($theme === "dark");
</script>

<button
  type="button"
  onclick={toggleTheme}
  aria-pressed={dark}
  aria-label="Toggle dark mode"
  title={dark ? "Dark mode (switch to light)" : "Light mode (switch to dark)"}
  >{dark ? "☀" : "☾"}</button
>
