const KEY = '__bass_cursor__';

export class CursorStore {
  private storage: Storage;
  private namespace: string;

  constructor(storage: Storage, appId: string) {
    this.storage = storage;
    this.namespace = `${KEY}:${appId}`;
  }

  load(): number {
    const raw = this.storage.getItem(this.namespace);
    if (!raw) return 0;
    const n = Number(raw);
    return Number.isFinite(n) ? n : 0;
  }

  save(v: number): void {
    this.storage.setItem(this.namespace, String(v));
  }

  clear(): void {
    this.storage.removeItem(this.namespace);
  }
}
