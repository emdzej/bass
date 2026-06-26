<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import { bass, APP } from '$lib/bass';

  let outboxJson = $state('');
  let cursor = $state(0);
  let tokens = $state('');

  function refresh() {
    if (!browser) return;
    outboxJson = localStorage.getItem(`__bass_outbox__:${APP}`) ?? '(empty)';
    cursor = Number(localStorage.getItem(`__bass_cursor__:${APP}`) ?? 0);
    const t = localStorage.getItem(`__bass_tokens__:${APP}`);
    if (t) {
      const parsed = JSON.parse(t) as { deviceId: string; expiresAt: number };
      tokens = `device ${parsed.deviceId.slice(0, 12)}… expires ${new Date(
        parsed.expiresAt,
      ).toLocaleString()}`;
    } else {
      tokens = '(no tokens stored)';
    }
  }

  onMount(() => {
    refresh();
    const id = setInterval(refresh, 1000);
    return () => clearInterval(id);
  });

  async function flushNow() {
    await bass().flush();
    refresh();
  }
</script>

<h1>Inspector</h1>
<button onclick={flushNow}>flush outbox now</button>
<button onclick={refresh}>refresh</button>

<section class="card">
  <h2>cursor</h2>
  <code>{cursor}</code>
</section>
<section class="card">
  <h2>tokens</h2>
  <code>{tokens}</code>
</section>
<section class="card">
  <h2>outbox</h2>
  <pre>{outboxJson}</pre>
</section>

<style>
  .card {
    background: light-dark(#fff, #1a1a1a);
    border: 1px solid light-dark(#e2e2e2, #2a2a2a);
    border-radius: 0.5rem;
    padding: 1rem;
    margin: 1rem 0;
  }
  pre {
    white-space: pre-wrap;
    word-break: break-all;
    font-size: 0.85rem;
  }
  button {
    margin-right: 0.5rem;
  }
</style>
