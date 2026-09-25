<script lang="ts">
  function login() {
    window.location.href = "/api/auth/login";
  }
</script>

<main class="login">
  <section class="login-card">
    <h1>APEKEEPER</h1>
    <p>Access the expedition ledger for Ape Enclosure.</p>
    <button class="gold" onclick={login}>Sign in with Battle.net</button><small
      >GM and officer approval roles apply after sign-in.</small
    >
  </section>
</main>
