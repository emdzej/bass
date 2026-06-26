import { writable, type Readable, type Writable } from 'svelte/store';
import type { AuthState, BassClient } from '@emdzej/bass-client';

/**
 * bassWritable returns a Svelte writable store backed by a bass-synced key.
 * Reads come from the local cache via bass.subscribe; writes go through
 * bass.set so they're pushed to the server (debounced via the outbox).
 *
 * The store value is always a string — match how localStorage works. If you
 * need structured data, JSON.stringify on the way in and JSON.parse on the
 * way out, or use the manual API for that.
 */
export function bassWritable(
  bass: BassClient,
  key: string,
  defaultValue: string,
): Writable<string> {
  const initial = bass.get(key) ?? defaultValue;
  const inner = writable<string>(initial);

  // Flag suppresses the round-trip when an inbound change updates the store.
  let applyingRemote = false;
  bass.subscribe(key, (v) => {
    applyingRemote = true;
    try {
      inner.set(v ?? defaultValue);
    } finally {
      applyingRemote = false;
    }
  });

  return {
    subscribe: inner.subscribe,
    set(v: string) {
      inner.set(v);
      if (!applyingRemote) void bass.set(key, v);
    },
    update(fn: (v: string) => string) {
      inner.update((cur) => {
        const next = fn(cur);
        if (!applyingRemote) void bass.set(key, next);
        return next;
      });
    },
  };
}

/**
 * Read-only version — writes are not exposed. Useful for derived/server-driven
 * keys that should never be written from this client.
 */
export function bassReadable(
  bass: BassClient,
  key: string,
  defaultValue: string,
): Readable<string> {
  const initial = bass.get(key) ?? defaultValue;
  const inner = writable<string>(initial);
  bass.subscribe(key, (v) => inner.set(v ?? defaultValue));
  return { subscribe: inner.subscribe };
}

/**
 * useBassAuth returns a Readable<AuthState> that re-emits whenever pairing,
 * refresh, or unpair changes the auth state.
 */
export function useBassAuth(bass: BassClient): Readable<AuthState> {
  const inner = writable<AuthState>(bass.authState());
  bass.onAuthChange((state) => inner.set(state));
  return { subscribe: inner.subscribe };
}
