const KEY = '__bass_tokens__';

export interface TokenSet {
  deviceId: string;
  syncToken: string;
  refreshToken: string;
  expiresAt: number; // epoch ms
}

export class TokenStore {
  private storage: Storage;
  private namespace: string;

  constructor(storage: Storage, appId: string) {
    this.storage = storage;
    this.namespace = `${KEY}:${appId}`;
  }

  load(): TokenSet | null {
    try {
      const raw = this.storage.getItem(this.namespace);
      if (!raw) return null;
      return JSON.parse(raw) as TokenSet;
    } catch {
      return null;
    }
  }

  save(t: TokenSet): void {
    this.storage.setItem(this.namespace, JSON.stringify(t));
  }

  clear(): void {
    this.storage.removeItem(this.namespace);
  }
}
