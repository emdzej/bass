<script lang="ts">
  import { browser } from '$app/environment';
  import { bass } from '$lib/bass';

  async function startPair(mode: 'redirect' | 'popup') {
    if (!browser) return;
    await bass().pair({
      redirectUri: location.origin + '/sync-cb',
      mode,
      deviceLabel: navigator.userAgent.slice(0, 60),
    });
  }

  async function unpair() {
    if (!browser) return;
    await bass().unpair();
    location.reload();
  }
</script>

<h1>backendless app state sync demo</h1>
<p>
  This app exercises <code>@emdzej/bass-client</code> and <code>@emdzej/bass-svelte</code> against
  a locally running <code>bass</code> service.
</p>

<section class="card">
  <h2>1. Pair this device</h2>
  <p>Choose a transport mode. Both end at <code>/sync-cb</code>.</p>
  <button onclick={() => startPair('redirect')}>Pair (redirect)</button>
  <button onclick={() => startPair('popup')}>Pair (popup)</button>
  <button class="danger" onclick={unpair}>Unpair / clear local tokens</button>
</section>

<section class="card">
  <h2>2. Try the settings page</h2>
  <p>
    <a href="/settings">Open settings</a> — flip values in two tabs and watch them converge.
  </p>
</section>

<section class="card">
  <h2>3. Inspect the client state</h2>
  <p>
    <a href="/inspector">Open inspector</a> for outbox + cursor + WS state.
  </p>
</section>

<style>
  .card {
    background: light-dark(#fff, #1a1a1a);
    border: 1px solid light-dark(#e2e2e2, #2a2a2a);
    border-radius: 0.5rem;
    padding: 1rem 1.25rem;
    margin: 1rem 0;
  }
  button {
    padding: 0.5rem 1rem;
    margin-right: 0.5rem;
  }
  .danger {
    color: crimson;
  }
</style>
