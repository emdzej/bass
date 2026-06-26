const KEY = '__bass_outbox__';

export interface OutboxEntry {
  key: string;
  value: string | null; // base64; null for deletes
  deleted: boolean;
  baseVersion: number;
  queuedAt: number;
}

/**
 * Outbox holds pending writes that haven't been flushed to the server yet.
 * Entries are keyed by sync key, so a chatty app coalesces naturally — only
 * the latest write per key is kept. Persisted to storage so writes survive
 * a tab close / reload.
 */
export class Outbox {
  private storage: Storage;
  private namespace: string;
  private cache: Map<string, OutboxEntry>;

  constructor(storage: Storage, appId: string) {
    this.storage = storage;
    this.namespace = `${KEY}:${appId}`;
    this.cache = this.load();
  }

  private load(): Map<string, OutboxEntry> {
    try {
      const raw = this.storage.getItem(this.namespace);
      if (!raw) return new Map();
      const obj = JSON.parse(raw) as Record<string, OutboxEntry>;
      return new Map(Object.entries(obj));
    } catch {
      return new Map();
    }
  }

  private persist(): void {
    if (this.cache.size === 0) {
      this.storage.removeItem(this.namespace);
      return;
    }
    const obj: Record<string, OutboxEntry> = {};
    for (const [k, v] of this.cache) obj[k] = v;
    this.storage.setItem(this.namespace, JSON.stringify(obj));
  }

  queue(entry: Omit<OutboxEntry, 'queuedAt'>): void {
    this.cache.set(entry.key, { ...entry, queuedAt: Date.now() });
    this.persist();
  }

  snapshot(): OutboxEntry[] {
    return Array.from(this.cache.values());
  }

  ack(keys: string[]): void {
    for (const k of keys) this.cache.delete(k);
    this.persist();
  }

  clear(): void {
    this.cache.clear();
    this.persist();
  }

  size(): number {
    return this.cache.size;
  }
}
