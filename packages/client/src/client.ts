import { Outbox } from './outbox.js';
import { completePairingFromUrl, pair } from './pairing.js';
import { attachProxy, compileKeyMatcher, decodeValue, encodeValue } from './proxy.js';
import { CursorStore } from './storage/cursor.js';
import { TokenStore } from './storage/tokens.js';
import { type PushItem, RestTransport } from './transport/rest.js';
import { WSChannel } from './transport/ws.js';
import type {
  AuthState,
  BassClientOptions,
  BassDevice,
  DiscoveryConfig,
  HydrateOptions,
  HydrateResult,
  PairOptions,
} from './types.js';

type Listener<T> = (value: T) => void;

/**
 * Main bass client. Created via `createBassClient(options)`.
 */
export class BassClient {
  private opts: Required<Pick<BassClientOptions, 'debounceMs'>> & BassClientOptions;
  private storage: Storage;
  private tokens: TokenStore;
  private cursor: CursorStore;
  private outbox: Outbox;
  private rest: RestTransport;
  private fetchImpl: typeof fetch;
  private WSImpl: typeof WebSocket;
  private matches: (key: string) => boolean;
  private detachProxy: (() => void) | null = null;
  private ws: WSChannel | null = null;
  private discoveryCache: DiscoveryConfig | null = null;
  private flushTimer: ReturnType<typeof setTimeout> | null = null;
  private flushing = false;
  private subscribers: Map<string, Set<Listener<string | null>>> = new Map();
  private authSubscribers = new Set<Listener<AuthState>>();
  private storageListener: ((ev: StorageEvent) => void) | null = null;

  constructor(opts: BassClientOptions) {
    this.opts = { debounceMs: 500, ...opts };
    this.storage = opts.storage ?? window.localStorage;
    this.fetchImpl = opts.fetchImpl ?? globalThis.fetch.bind(globalThis);
    this.WSImpl = opts.WebSocketImpl ?? (globalThis.WebSocket as typeof WebSocket);
    this.tokens = new TokenStore(this.storage, opts.appId);
    this.cursor = new CursorStore(this.storage, opts.appId);
    this.outbox = new Outbox(this.storage, opts.appId);
    this.matches = compileKeyMatcher(opts.keys);
    this.rest = new RestTransport(
      opts.serviceUrl.replace(/\/$/, ''),
      this.tokens,
      this.fetchImpl,
      () => this.notifyAuth(),
    );
  }

  // ─── auth state ────────────────────────────────────────────────────────

  isPaired(): boolean {
    const t = this.tokens.load();
    return !!t && t.expiresAt > Date.now();
  }

  authState(): AuthState {
    const t = this.tokens.load();
    if (!t) return { isPaired: false };
    return {
      isPaired: t.expiresAt > Date.now(),
      deviceId: t.deviceId,
      expiresAt: t.expiresAt,
    };
  }

  onAuthChange(cb: Listener<AuthState>): () => void {
    this.authSubscribers.add(cb);
    cb(this.authState());
    return () => this.authSubscribers.delete(cb);
  }

  private notifyAuth(): void {
    const state = this.authState();
    for (const s of this.authSubscribers) s(state);
  }

  // ─── pairing ───────────────────────────────────────────────────────────

  async pair(opts: PairOptions): Promise<void> {
    await pair(this.opts.serviceUrl, this.opts.appId, this.tokens, opts);
    this.notifyAuth();
  }

  completePairingFromUrl(): boolean {
    const ok = completePairingFromUrl(this.tokens);
    if (ok) this.notifyAuth();
    return ok;
  }

  async unpair(): Promise<void> {
    const t = this.tokens.load();
    if (t) {
      try {
        await this.rest.unpair(t.deviceId);
      } catch {
        // best effort
      }
    }
    this.tokens.clear();
    this.cursor.clear();
    this.outbox.clear();
    if (this.ws) this.ws.stop();
    this.notifyAuth();
  }

  // ─── discovery ─────────────────────────────────────────────────────────

  async discovery(): Promise<DiscoveryConfig> {
    if (this.discoveryCache) return this.discoveryCache;
    this.discoveryCache = await this.rest.discovery();
    return this.discoveryCache;
  }

  // ─── manual KV API ─────────────────────────────────────────────────────

  /**
   * Synchronous read from the local cache. Returns whatever is in
   * localStorage right now. Triggers a background sync if the cursor is
   * stale, but does not wait for it.
   */
  get(key: string): string | null {
    return this.storage.getItem(key);
  }

  /**
   * Write a value locally and queue it for upstream sync.
   */
  async set(key: string, value: string): Promise<void> {
    this.storage.setItem(key, value);
    this.queueWrite(key, value, false);
  }

  async delete(key: string): Promise<void> {
    this.storage.removeItem(key);
    this.queueWrite(key, null, true);
  }

  subscribe(key: string, listener: Listener<string | null>): () => void {
    let set = this.subscribers.get(key);
    if (!set) {
      set = new Set();
      this.subscribers.set(key, set);
    }
    set.add(listener);
    listener(this.storage.getItem(key));
    return () => {
      const s = this.subscribers.get(key);
      if (!s) return;
      s.delete(listener);
      if (s.size === 0) this.subscribers.delete(key);
    };
  }

  private emitLocal(key: string, value: string | null): void {
    const set = this.subscribers.get(key);
    if (!set) return;
    for (const cb of set) cb(value);
  }

  // ─── proxy ─────────────────────────────────────────────────────────────

  attachLocalStorageProxy(): void {
    if (this.detachProxy) return;
    this.detachProxy = attachProxy({
      storage: this.storage,
      matches: this.matches,
      onWrite: (key, value, deleted) => this.queueWrite(key, value, deleted),
    });
    // Cross-tab: another tab changed localStorage. Re-emit to subscribers.
    if (typeof window !== 'undefined') {
      this.storageListener = (ev: StorageEvent) => {
        if (ev.storageArea !== this.storage) return;
        if (ev.key === null) return;
        if (this.matches(ev.key)) this.emitLocal(ev.key, ev.newValue);
      };
      window.addEventListener('storage', this.storageListener);
    }
  }

  detachLocalStorageProxy(): void {
    if (this.detachProxy) {
      this.detachProxy();
      this.detachProxy = null;
    }
    if (this.storageListener && typeof window !== 'undefined') {
      window.removeEventListener('storage', this.storageListener);
      this.storageListener = null;
    }
  }

  // ─── outbox & flush ────────────────────────────────────────────────────

  private queueWrite(key: string, value: string | null, deleted: boolean): void {
    const baseVersion = this.cursor.load();
    this.outbox.queue({
      key,
      value: value === null ? null : encodeValue(value),
      deleted,
      baseVersion,
    });
    this.emitLocal(key, value);
    this.scheduleFlush();
  }

  private scheduleFlush(): void {
    if (this.flushTimer) return;
    this.flushTimer = setTimeout(() => {
      this.flushTimer = null;
      void this.flush();
    }, this.opts.debounceMs);
  }

  /**
   * Manually push all queued writes. Called automatically by the debounce
   * timer; expose it for tests and "flush before unload" use cases.
   */
  async flush(): Promise<void> {
    if (this.flushing) return;
    if (!this.isPaired()) return;
    const entries = this.outbox.snapshot();
    if (entries.length === 0) return;
    this.flushing = true;
    try {
      const items: PushItem[] = entries.map((e) => ({
        key: e.key,
        value: e.value ?? undefined,
        base_version: e.baseVersion,
        deleted: e.deleted,
        payload_ver: 1,
      }));
      const res = await this.rest.push(items);
      this.outbox.ack(entries.map((e) => e.key));
      this.cursor.save(res.cursor);
    } catch {
      // Leave entries queued; we'll retry on the next debounce or reconnect.
    } finally {
      this.flushing = false;
    }
  }

  // ─── hydrate ───────────────────────────────────────────────────────────

  async hydrate(opts: HydrateOptions = {}): Promise<HydrateResult> {
    const timeoutMs = opts.timeoutMs ?? 2000;
    if (!this.isPaired()) return { timedOut: false, itemsApplied: 0 };
    const pullPromise = this.runPull();
    if (timeoutMs === 0) {
      // Non-blocking: kick off the pull but don't wait.
      void pullPromise;
      return { timedOut: false, itemsApplied: 0 };
    }
    if (!Number.isFinite(timeoutMs)) {
      return await pullPromise;
    }
    return await Promise.race<HydrateResult>([
      pullPromise,
      new Promise<HydrateResult>((resolve) =>
        setTimeout(() => resolve({ timedOut: true, itemsApplied: 0 }), timeoutMs),
      ),
    ]);
  }

  private async runPull(): Promise<HydrateResult> {
    let total = 0;
    let cursor = this.cursor.load();
    // Loop on has_more to drain the backlog.
    // eslint-disable-next-line no-constant-condition
    while (true) {
      const res = await this.rest.pull(cursor);
      for (const item of res.items) {
        if (item.deleted) {
          this.storage.removeItem(item.key);
          this.emitLocal(item.key, null);
        } else {
          const v = decodeValue(item.value);
          this.storage.setItem(item.key, v);
          this.emitLocal(item.key, v);
        }
        total++;
      }
      cursor = res.cursor;
      this.cursor.save(cursor);
      if (!res.has_more) break;
    }
    return { timedOut: false, itemsApplied: total };
  }

  // ─── WS notifications ──────────────────────────────────────────────────

  async startNotifications(): Promise<void> {
    if (this.ws) return;
    const cfg = await this.discovery();
    this.ws = new WSChannel(
      cfg.endpoints.changes_ws,
      this.tokens,
      {
        onChange: (cursor) => {
          if (cursor > this.cursor.load()) void this.runPull();
        },
      },
      () => this.cursor.load(),
      this.WSImpl,
    );
    this.ws.start();
  }

  stopNotifications(): void {
    if (this.ws) {
      this.ws.stop();
      this.ws = null;
    }
  }

  // ─── devices ───────────────────────────────────────────────────────────

  devices = {
    list: (): Promise<{ devices: BassDevice[]; current: string }> => this.rest.listDevices(),
    revoke: (deviceId: string): Promise<void> => this.rest.unpair(deviceId),
  };
}

export function createBassClient(opts: BassClientOptions): BassClient {
  return new BassClient(opts);
}
