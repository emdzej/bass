<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import { bass } from '$lib/bass';
  import type { BassDevice } from '@emdzej/bass-client';

  let devices = $state<BassDevice[]>([]);
  let current = $state<string>('');
  let loading = $state(true);
  let err = $state<string | null>(null);

  onMount(() => {
    if (!browser) return;
    void load();
  });

  async function load() {
    loading = true;
    err = null;
    try {
      const res = await bass().devices.list();
      devices = res.devices;
      current = res.current;
    } catch (e) {
      err = (e as Error).message;
    } finally {
      loading = false;
    }
  }

  async function revoke(id: string) {
    if (!confirm('Revoke this device?')) return;
    await bass().devices.revoke(id);
    await load();
  }
</script>

<h1>Devices</h1>

{#if loading}
  <p>loading…</p>
{:else if err}
  <p class="err">error: {err}</p>
{:else}
  <table>
    <thead>
      <tr>
        <th>id</th>
        <th>label</th>
        <th>last seen</th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each devices as d (d.id)}
        <tr class:current={d.id === current}>
          <td><code>{d.id}</code></td>
          <td>{d.label ?? '—'}</td>
          <td>{new Date(d.last_seen_at).toLocaleString()}</td>
          <td>
            {#if !d.revoked}
              <button onclick={() => revoke(d.id)}>revoke</button>
            {:else}
              <em>revoked</em>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    text-align: left;
    padding: 0.5rem;
    border-bottom: 1px solid light-dark(#eee, #2a2a2a);
  }
  tr.current {
    background: light-dark(#f4faff, #1a2a3a);
  }
  .err {
    color: crimson;
  }
</style>
