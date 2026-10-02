<script lang="ts">
  let signingIn = $state(false);
  let error = $state("");

  function login() {
    signingIn = true;
    error = "";
    try {
      window.location.href = "/api/auth/login";
    } catch {
      signingIn = false;
      error = "Sign-in could not be started. Please try again.";
    }
  }
</script>

<main class="login">
  <section class="login-card">
    <h1>APEKEEPER</h1>
    <p>Access the expedition ledger for Ape Enclosure.</p>
    {#if error}<p class="error-state" role="alert">{error}</p>{/if}
    <p class="login-status" role="status" aria-live="polite" aria-atomic="true">
      {signingIn ? "Opening Battle.net sign-in…" : ""}
    </p>
    <button class="gold" disabled={signingIn} onclick={login}
      >{signingIn ? "Opening sign-in…" : "Sign in with Battle.net"}</button
    ><small>GM and officer approval roles apply after sign-in.</small>
  </section>
</main>
