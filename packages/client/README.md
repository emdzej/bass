# @emdzej/bass-client

Client library for **[bass](https://github.com/emdzej/bass)** — backendless
app state synchronization.

Drop-in `localStorage` sync across a user's devices for apps that
intentionally have no backend. Works fully offline / unpaired (no-op),
opts into sync when the user pairs once via OIDC.

```sh
pnpm add @emdzej/bass-client
```

## Quick start

```ts
import { createBassClient } from '@emdzej/bass-client';

const bass = createBassClient({
  serviceUrl: 'https://bass.example.com',
  appId: 'my-app',
  keys: ['myapp-*'],           // optional, default ['*']
});

// One-time pairing (call from a "sync settings" button)
await bass.pair({
  redirectUri: location.origin + '/sync-cb',
  mode: 'redirect',            // or 'popup'
});

// In your /sync-cb route, finish the pairing
bass.completePairingFromUrl();

// On boot, hydrate local cache from server before mounting UI
if (bass.isPaired()) {
  await bass.hydrate({ timeoutMs: 2000 });
}

// Pick one (or both):

// (a) transparent proxy — keep using window.localStorage
bass.attachLocalStorageProxy();

// (b) manual API
await bass.set('myapp-theme', 'dark');
const v = bass.get('myapp-theme');
const unsub = bass.subscribe('myapp-theme', (v) => render(v));

// Start the WS notification channel for live updates from other devices
await bass.startNotifications();
```

## Why two APIs?

- **Manual** — explicit, fits state libraries (Redux, Zustand, Pinia, Svelte stores).
- **Proxy** — zero-touch. Existing `localStorage.setItem(...)` calls just work.

Pick whichever suits your codebase. They can coexist.

## Reading

Reads always come from `localStorage` synchronously. The library never
blocks a read on the network — `bass.get(key)` and `localStorage.getItem(key)`
behave identically.

## Writing

Writes go to:
1. `localStorage` immediately (so the next read sees them).
2. An outbox keyed by sync key (latest write per key wins — chatty apps
   coalesce naturally).

The outbox drains on a debounce timer, on reconnect, and on `bass.flush()`.
Entries survive a tab close (they live in `localStorage` themselves).

## Offline / unpaired

If `bass.isPaired()` is `false`, the proxy is a passthrough and the manual
API just hits `localStorage`. Apps don't need to branch on auth state —
they keep working with or without sync.

## Pairing modes

| Mode | Behaviour | When to use |
|---|---|---|
| `redirect` | Whole-page navigation to the IdP, redirect back to your `/sync-cb`. | Default. Mobile-safe, no popup blockers. |
| `popup` | `window.open` to the IdP. Tokens postMessage'd back to opener. | Desktop UX. Falls back to redirect when popups are blocked. |

The `/sync-cb` route in your app calls `bass.completePairingFromUrl()` —
the same line handles both modes.

## Full reference

See [the bass docs](https://github.com/emdzej/bass/blob/main/docs/API.md)
for the underlying REST + WS protocol, and
[`SPEC.md`](https://github.com/emdzej/bass/blob/main/SPEC.md) for the
design rationale.

## License

MIT
