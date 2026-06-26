<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import { bass } from '$lib/bass';
  import { bassWritable } from '@emdzej/bass-svelte';

  let theme = $state<'light' | 'dark'>('light');
  let scale = $state(100);
  let note = $state('');

  // Plain "manual API" panel
  let manualTheme = $state('light');
  let manualNote = $state('');

  onMount(() => {
    if (!browser) return;
    const b = bass();

    // — adapter wiring —
    const themeStore = bassWritable(b, 'demo-theme', 'light');
    const scaleStore = bassWritable(b, 'demo-scale', '100');
    const noteStore = bassWritable(b, 'demo-note', '');
    const unsubs = [
      themeStore.subscribe((v) => (theme = (v as 'light' | 'dark') ?? 'light')),
      scaleStore.subscribe((v) => (scale = Number(v) || 100)),
      noteStore.subscribe((v) => (note = v ?? '')),
    ];

    // — manual API panel —
    const unsubManual = [
      b.subscribe('demo-manual-theme', (v) => (manualTheme = v ?? 'light')),
      b.subscribe('demo-manual-note', (v) => (manualNote = v ?? '')),
    ];

    // assignments push to bass via the wrappers above
    $effect(() => {
      themeStore.set(theme);
    });
    $effect(() => {
      scaleStore.set(String(scale));
    });
    $effect(() => {
      noteStore.set(note);
    });

    return () => {
      for (const u of unsubs) u();
      for (const u of unsubManual) u();
    };
  });

  function setManualTheme(v: string) {
    void bass().set('demo-manual-theme', v);
  }
  function setManualNote(v: string) {
    void bass().set('demo-manual-note', v);
  }
  function deleteManual() {
    void bass().delete('demo-manual-note');
  }
</script>

<h1>Settings</h1>

<div class="grid">
  <section class="card">
    <h2>via Svelte adapter (bassWritable)</h2>

    <label>
      Theme
      <select bind:value={theme}>
        <option value="light">light</option>
        <option value="dark">dark</option>
      </select>
    </label>

    <label>
      Label scale: {scale}%
      <input type="range" min="50" max="200" bind:value={scale} />
    </label>

    <label>
      Note
      <input type="text" bind:value={note} placeholder="type something…" />
    </label>
  </section>

  <section class="card">
    <h2>via manual API</h2>

    <label>
      Theme
      <select value={manualTheme} onchange={(e) => setManualTheme(e.currentTarget.value)}>
        <option value="light">light</option>
        <option value="dark">dark</option>
      </select>
    </label>

    <label>
      Note
      <input
        type="text"
        value={manualNote}
        oninput={(e) => setManualNote(e.currentTarget.value)}
      />
    </label>
    <button onclick={deleteManual}>delete note</button>
  </section>
</div>

<p class="hint">
  Open this page in another tab or another browser — values converge within a second of each
  write (or earlier when the WS notification arrives).
</p>

<style>
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }
  .card {
    background: light-dark(#fff, #1a1a1a);
    border: 1px solid light-dark(#e2e2e2, #2a2a2a);
    border-radius: 0.5rem;
    padding: 1rem;
  }
  label {
    display: flex;
    flex-direction: column;
    margin: 0.5rem 0;
    font-size: 0.9rem;
  }
  input,
  select {
    margin-top: 0.25rem;
  }
  .hint {
    color: light-dark(#555, #aaa);
    font-size: 0.85rem;
  }
</style>
