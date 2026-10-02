<script lang="ts">
  import { onMount } from "svelte";
  import { match, navigate, type Route } from "$lib/router";
  import { client } from "$lib/api";
  import { currentUser } from "$lib/stores";
  import Logo from "$components/Logo.svelte";
  import ThemeToggle from "$components/ThemeToggle.svelte";
  import Dashboard from "$routes/Dashboard.svelte";
  import Roster from "$routes/Roster.svelte";
  import CharacterDetail from "$routes/CharacterDetail.svelte";
  import Sync from "$routes/Sync.svelte";
  import Officer from "$routes/Officer.svelte";
  import Memberships from "$routes/Memberships.svelte";
  import Login from "$routes/Login.svelte";
  import NotFound from "$routes/NotFound.svelte";
  const routes: Route[] = [
    { path: "/", component: Dashboard },
    { path: "/roster", component: Roster },
    { path: "/characters/{name}", component: CharacterDetail },
    { path: "/sync", component: Sync },
    { path: "/officer", component: Officer },
    { path: "/members", component: Memberships },
    { path: "/login", component: Login },
  ];
  let tick = $state(0);
  let guilds = $state<
    { id: number; slug: string; name: string; realm: string; region: string; role: string }[]
  >([]);
  let selectedGuild = $state("");
  let confirmedGuild = $state("");
  let guildSwitching = $state(false);
  let bootstrapLoading = $state(true);
  let actionError = $state("");
  let routeAnnouncement = $state("");
  let mainElement = $state<HTMLElement | null>(null);
  let active = $derived.by(() => {
    tick;
    return match(routes);
  });
  const routeLabel = (path: string) =>
    ({
      "/": "The Enclosure",
      "/roster": "Roster",
      "/sync": "Keeper Controls",
      "/officer": "Officer workflow",
      "/members": "Memberships",
      "/characters/{name}": "Character detail",
    })[path] ?? "Page";
  $effect(() => {
    const path = active?.route.path;
    if (!path || bootstrapLoading || path === "/login") return;
    routeAnnouncement = `${routeLabel(path)} loaded`;
    queueMicrotask(() => mainElement?.focus());
  });
  onMount(() => {
    const listener = () => tick++;
    addEventListener("popstate", listener);
    client
      .me()
      .then((u) => {
        currentUser.set(u);
        return client.guilds().then((items) => {
          guilds = items;
          confirmedGuild = items.find((item) => item.selected)?.slug ?? items[0]?.slug ?? "";
          selectedGuild = confirmedGuild;
        });
      })
      .then(() => {
        if (location.pathname === "/login") navigate("/");
      })
      .catch(() => {
        if (location.pathname !== "/login") navigate("/login");
      })
      .finally(() => (bootstrapLoading = false));
    return () => removeEventListener("popstate", listener);
  });
  async function logout() {
    actionError = "";
    try {
      await client.logout();
      currentUser.set(null);
      navigate("/login");
    } catch (error) {
      actionError = error instanceof Error ? error.message : "Could not log out. Please try again.";
    }
  }
  async function switchGuild() {
    if (!selectedGuild) return;
    if (selectedGuild === confirmedGuild) return;
    actionError = "";
    guildSwitching = true;
    const requestedGuild = selectedGuild;
    try {
      await client.selectGuild(requestedGuild);
      confirmedGuild = requestedGuild;
      location.reload();
    } catch (error) {
      selectedGuild = confirmedGuild;
      actionError =
        error instanceof Error ? error.message : "Could not switch guild. Please try again.";
    } finally {
      guildSwitching = false;
    }
  }
</script>

{#if active?.route.path === "/login"}<Login />{:else if bootstrapLoading}<main
    class="login"
    aria-live="polite"
  >
    <p class="skeleton" role="status">Opening expedition ledger…</p>
  </main>{:else}<div class="shell">
    <aside>
      <a href="/" class="brand"><Logo /><span>APEKEEPER<small>Ape Enclosure</small></span></a>
      <nav>
        <a href="/">The Enclosure</a><a href="/roster">Roster</a><a href="/officer"
          >Officer workflow</a
        ><a href="/sync">Keeper Controls</a><a href="/members">Memberships</a>
      </nav>
    </aside>
    <main bind:this={mainElement} tabindex="-1">
      <p class="sr-only" role="status" aria-live="polite" aria-atomic="true">{routeAnnouncement}</p>
      <header class="topbar">
        <span>EXPEDITION LOG / {new Date().toLocaleDateString()}</span>
        <span class="topbar-actions">
          {#if $currentUser}<span
              >{#if guilds.length > 1}<select
                  bind:value={selectedGuild}
                  disabled={guildSwitching}
                  onchange={switchGuild}
                  aria-label="Current guild"
                  >{#each guilds as guild}<option value={guild.slug}>{guild.name}</option
                    >{/each}</select
                >{/if}
              {$currentUser.battletag}
              <b
                >{$currentUser.appRole === "superadmin"
                  ? "Super Admin"
                  : $currentUser.appRole === "admin"
                    ? "Guild Master"
                    : $currentUser.appRole === "officer"
                      ? "Officer"
                      : "Ape"}</b
              > <button onclick={logout}>Logout</button></span
            >{/if}
          <ThemeToggle />
        </span>
      </header>
      {#if actionError}<p class="error-state" role="alert">{actionError}</p>{/if}
      {#if active?.route.path === "/"}<Dashboard
        />{:else if active?.route.path === "/roster"}<Roster
        />{:else if active?.route.path === "/sync"}<Sync
        />{:else if active?.route.path === "/officer"}<Officer
        />{:else if active?.route.path === "/members"}<Memberships
        />{:else if active?.route.path === "/characters/{name}"}<CharacterDetail
          name={active.params.name}
        />{:else}<NotFound />{/if}
    </main>
  </div>{/if}
