<script lang="ts">
  import { onMount } from "svelte";
  import { match, navigate, type Route } from "$lib/router";
  import { client } from "$lib/api";
  import { currentUser } from "$lib/stores";
  import Logo from "$components/Logo.svelte";
  import Dashboard from "$routes/Dashboard.svelte";
  import Roster from "$routes/Roster.svelte";
  import CharacterDetail from "$routes/CharacterDetail.svelte";
  import Sync from "$routes/Sync.svelte";
  import Login from "$routes/Login.svelte";
  import NotFound from "$routes/NotFound.svelte";
  const routes: Route[] = [
    { path: "/", component: Dashboard },
    { path: "/roster", component: Roster },
    { path: "/characters/{id}", component: CharacterDetail },
    { path: "/sync", component: Sync },
    { path: "/login", component: Login },
  ];
  let tick = $state(0);
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
</script>

{#if active?.route.path === "/login"}<Login />{:else}<div class="shell">
    <aside>
      <a href="/" class="brand"><Logo /><span>APEKEEPER<small>Ape Enclosure</small></span></a>
      <nav>
        <a href="/">The Enclosure</a><a href="/roster">Roster</a><a href="/sync">Keeper Controls</a>
      </nav>
    </aside>
    <main>
      <header class="topbar">
        <span>EXPEDITION LOG / {new Date().toLocaleDateString()}</span>{#if $currentUser}<span
            >{$currentUser.battletag}
            <b
              >{$currentUser.appRole === "superadmin"
                ? "Super Admin"
                : $currentUser.appRole === "admin"
                  ? "Guild Master"
                  : $currentUser.appRole === "officer"
                    ? "Officer"
                    : "Member"}</b
            > <button onclick={logout}>Logout</button></span
          >{/if}
      </header>
      {#if active?.route.path === "/"}<Dashboard
        />{:else if active?.route.path === "/roster"}<Roster
        />{:else if active?.route.path === "/sync"}<Sync
        />{:else if active?.route.path === "/characters/{id}"}<CharacterDetail
          id={active.params.id}
        />{:else}<NotFound />{/if}
    </main>
  </div>{/if}
