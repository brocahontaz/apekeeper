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
  let active = $derived.by(() => {
    tick;
    return match(routes);
  });
  onMount(() => {
    const listener = () => tick++;
    addEventListener("popstate", listener);
    client
      .me()
      .then((u) => {
        currentUser.set(u);
        client.guilds().then((items) => {
          guilds = items;
          selectedGuild = items.find((item) => item.selected)?.slug ?? items[0]?.slug ?? "";
        });
        if (location.pathname === "/login") navigate("/");
      })
      .catch(() => {
        if (location.pathname !== "/login") navigate("/login");
      });
    return () => removeEventListener("popstate", listener);
  });
  async function logout() {
    await client.logout();
    currentUser.set(null);
    navigate("/login");
  }
  async function switchGuild() {
    if (!selectedGuild) return;
    await client.selectGuild(selectedGuild);
    location.reload();
  }
</script>

{#if active?.route.path === "/login"}<Login />{:else}<div class="shell">
    <aside>
      <a href="/" class="brand"><Logo /><span>APEKEEPER<small>Ape Enclosure</small></span></a>
      <nav>
        <a href="/">The Enclosure</a><a href="/roster">Roster</a><a href="/officer"
          >Officer workflow</a
        ><a href="/sync">Keeper Controls</a><a href="/members">Memberships</a>
      </nav>
    </aside>
    <main>
      <header class="topbar">
        <span>EXPEDITION LOG / {new Date().toLocaleDateString()}</span>
        <span class="topbar-actions">
          {#if $currentUser}<span
              >{#if guilds.length > 1}<select
                  bind:value={selectedGuild}
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
