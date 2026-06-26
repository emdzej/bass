<script lang="ts">
  import { browser } from '$app/environment';
  import { onMount } from 'svelte';
  import { bass, SERVICE, APP } from '$lib/bass';
  import { useBassAuth } from '@emdzej/bass-svelte';

  let { children } = $props();

  let auth = $state({ isPaired: false, deviceId: undefined as string | undefined });

  onMount(() => {
    if (!browser) return;
    const b = bass();
    const authStore = useBassAuth(b);
    const unsub = authStore.subscribe((s) => {
      auth = { isPaired: s.isPaired, deviceId: s.deviceId };
    });
    if (b.isPaired()) {
      // Best-effort hydrate (non-blocking on cold start) before mounting.
      void b.hydrate({ timeoutMs: 2000 }).then(() => {
        b.attachLocalStorageProxy();
        void b.startNotifications();
      });
    }
    return unsub;
  });
</script>

<header>
  <strong>bass demo</strong>
  <nav>
    <a href="/">home</a>
    <a href="/settings">settings</a>
    <a href="/devices">devices</a>
    <a href="/inspector">inspector</a>
  </nav>
  <span class="status">
    {#if auth.isPaired}
      paired · {auth.deviceId?.slice(0, 8)}…
    {:else}
      not paired
    {/if}
  </span>
</header>
<main>
  {@render children()}
</main>
<footer>
  <code>{SERVICE}</code> · app <code>{APP}</code>
</footer>

<style>
  :global(body) {
    font-family: ui-sans-serif, system-ui, sans-serif;
    margin: 0;
    background: light-dark(#fafafa, #111);
    color: light-dark(#111, #eee);
  }
  header {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 1rem;
    border-bottom: 1px solid light-dark(#ddd, #333);
  }
  header nav {
    display: flex;
    gap: 1rem;
    flex: 1;
  }
  header nav a {
    color: inherit;
  }
  header .status {
    font-size: 0.85rem;
    opacity: 0.75;
  }
  main {
    max-width: 64rem;
    margin: 0 auto;
    padding: 1.5rem 1rem;
  }
  footer {
    padding: 0.75rem 1rem;
    font-size: 0.8rem;
    opacity: 0.6;
    border-top: 1px solid light-dark(#ddd, #333);
  }
</style>
