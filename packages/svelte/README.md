# @emdzej/bass-svelte

Svelte stores wrapping **[@emdzej/bass-client](https://www.npmjs.com/package/@emdzej/bass-client)**
so synced keys feel native in Svelte apps.

```sh
pnpm add @emdzej/bass-client @emdzej/bass-svelte
```

Peer deps: `svelte ^5`, `@emdzej/bass-client`.

## Usage

```svelte
<script lang="ts">
  import { bassWritable, useBassAuth } from '@emdzej/bass-svelte';
  import { bass } from '$lib/bass';

  const theme = bassWritable(bass(), 'myapp-theme', 'light');
  const auth = useBassAuth(bass());
</script>

<p>paired: {$auth.isPaired}</p>

<select bind:value={$theme}>
  <option value="light">light</option>
  <option value="dark">dark</option>
</select>
```

`$theme` reads from the local cache, assignments write through the bass
outbox. When another device pushes a change, the store re-emits and
components using `$theme` re-render.

## API

| Function | Returns | Purpose |
|---|---|---|
| `bassWritable(bass, key, default)` | `Writable<string>` | Two-way sync — reads from local cache, writes go through bass. |
| `bassReadable(bass, key, default)` | `Readable<string>` | Read-only sync — useful for derived/server-driven keys. |
| `useBassAuth(bass)` | `Readable<AuthState>` | Re-emits on pair / refresh / unpair. |

Values are strings (matching `localStorage`). For structured data,
`JSON.stringify` on the way in and `JSON.parse` on the way out — or use the
underlying manual API on the `BassClient` directly.

## Echo protection

When an inbound remote change updates the store, the wrapper suppresses
the round-trip back to bass to avoid an echo loop. Outbound writes (your
component setting `$theme = 'dark'`) push as expected.

## Full reference

See [the bass docs](https://github.com/emdzej/bass/blob/main/docs/API.md)
and [`@emdzej/bass-client`](https://www.npmjs.com/package/@emdzej/bass-client).

## License

MIT
