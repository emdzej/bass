<script lang="ts">
  import { onMount } from 'svelte';
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { bass } from '$lib/bass';

  let status = $state('completing pairing…');

  onMount(() => {
    if (!browser) return;
    const ok = bass().completePairingFromUrl();
    if (ok) {
      status = 'paired! redirecting…';
      // Popup flow: window.close() was already called inside the lib.
      // Redirect flow: bounce to /.
      if (!window.opener) {
        setTimeout(() => goto('/'), 400);
      }
    } else {
      status = 'no tokens in URL — pairing failed';
    }
  });
</script>

<h1>{status}</h1>
