<script lang="ts">
  import { client, type GuildMember, type MembershipCandidate } from "$lib/api";
  import { currentUser } from "$lib/stores";
  let members = $state<GuildMember[]>([]),
    error = $state(""),
    loading = $state(true),
    canManage = $state(false);
  let query = $state(""),
    candidates = $state<MembershipCandidate[]>([]),
    role = $state("member");
  async function load() {
    loading = true;
    error = "";
    try {
      const guilds = await client.guilds();
      const selected = guilds.find((g) => g.selected) ?? guilds[0];
      canManage =
        $currentUser?.appRole === "superadmin" ||
        selected?.role === "owner" ||
        selected?.role === "admin";
      if (canManage) members = await client.members();
    } catch (e) {
      error = e instanceof Error ? e.message : "Membership administration unavailable";
    } finally {
      loading = false;
    }
  }
  async function setRole(member: GuildMember, role: string) {
    try {
      await client.updateMember(member.id, role);
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : "Membership update failed";
    }
  }
  async function search() {
    try {
      candidates = await client.searchMembers(query);
    } catch (e) {
      error = e instanceof Error ? e.message : "Member search failed";
    }
  }
  async function grant(candidate: MembershipCandidate) {
    try {
      await client.inviteMember(candidate.battletag, role);
      query = "";
      candidates = [];
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : "Membership grant failed";
    }
  }
  async function remove(member: GuildMember) {
    if (!confirm(`Remove ${member.displayName || member.battletag} from this guild?`)) return;
    try {
      await client.removeMember(member.id);
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : "Membership removal failed";
    }
  }
  $effect(() => {
    if ($currentUser) void load();
  });
</script>

<h1>Guild memberships</h1>
{#if error}<p class="error" role="alert">{error}</p>{/if}
{#if loading}<p>Loading memberships…</p>{:else if !canManage}<p>
    Owner or administrator access is required.
  </p>{:else}
  <section aria-label="Grant membership">
    <h2>Grant membership</h2>
    <input bind:value={query} placeholder="BattleTag or display name" aria-label="Find user" />
    <button onclick={search}>Find</button>
    <select bind:value={role} aria-label="New member role">
      <option value="member">Member</option><option value="officer">Officer</option><option
        value="admin">Admin</option
      ><option value="owner">Owner</option>
    </select>
    {#each candidates as candidate}
      <p>
        {candidate.displayName || candidate.battletag} ({candidate.battletag})
        <button onclick={() => grant(candidate)}>Grant</button>
      </p>
    {/each}
  </section>
  <table>
    <thead><tr><th>Member</th><th>BattleTag</th><th>Role</th><th>Actions</th></tr></thead><tbody>
      {#each members as member}<tr
          ><td>{member.displayName}</td><td>{member.battletag}</td><td>{member.role}</td><td>
            <select
              aria-label={`Role for ${member.displayName}`}
              value={member.role}
              onchange={(e) => setRole(member, (e.currentTarget as HTMLSelectElement).value)}
            >
              <option value="member">Member</option><option value="officer">Officer</option><option
                value="admin">Admin</option
              ><option value="owner">Owner</option>
            </select><button onclick={() => remove(member)}>Remove</button>
          </td></tr
        >{/each}
    </tbody>
  </table>
{/if}
