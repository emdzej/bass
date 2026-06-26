import type { TokenStore } from '../storage/tokens.js';
import type { BassDevice, DiscoveryConfig, PullResponse, PushResponse } from '../types.js';

export class BassRestError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
    this.name = 'BassRestError';
  }
}

export class RestTransport {
  constructor(
    private serviceUrl: string,
    private tokens: TokenStore,
    private fetchImpl: typeof fetch,
    private onTokensRefreshed?: () => void,
  ) {}

  async discovery(): Promise<DiscoveryConfig> {
    const res = await this.fetchImpl(`${this.serviceUrl}/.well-known/bass-config`);
    if (!res.ok) throw await toError(res);
    return (await res.json()) as DiscoveryConfig;
  }

  async pull(since: number, limit?: number): Promise<PullResponse> {
    const url = new URL(`${this.serviceUrl}/v1/sync`);
    url.searchParams.set('since', String(since));
    if (limit) url.searchParams.set('limit', String(limit));
    return this.authed<PullResponse>(url.toString(), { method: 'GET' });
  }

  async push(items: PushItem[]): Promise<PushResponse> {
    return this.authed<PushResponse>(`${this.serviceUrl}/v1/sync`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ items }),
    });
  }

  async refresh(refreshToken: string): Promise<RefreshResponse> {
    const res = await this.fetchImpl(`${this.serviceUrl}/v1/token/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
    if (!res.ok) throw await toError(res);
    return (await res.json()) as RefreshResponse;
  }

  async listDevices(): Promise<{ devices: BassDevice[]; current: string }> {
    return this.authed<{ devices: BassDevice[]; current: string }>(
      `${this.serviceUrl}/v1/devices`,
      { method: 'GET' },
    );
  }

  async unpair(deviceId: string): Promise<void> {
    await this.authed<void>(`${this.serviceUrl}/v1/devices/${encodeURIComponent(deviceId)}`, {
      method: 'DELETE',
    });
  }

  private async authed<T>(url: string, init: RequestInit, retry = true): Promise<T> {
    const tokens = this.tokens.load();
    if (!tokens) throw new BassRestError(401, 'not_paired', 'no sync token available');
    const headers = new Headers(init.headers);
    headers.set('Authorization', `Bearer ${tokens.syncToken}`);
    const res = await this.fetchImpl(url, { ...init, headers });
    if (res.status === 401 && retry) {
      const ok = await this.tryRefresh();
      if (ok) return this.authed<T>(url, init, false);
    }
    if (!res.ok) throw await toError(res);
    if (res.status === 204) return undefined as unknown as T;
    return (await res.json()) as T;
  }

  private async tryRefresh(): Promise<boolean> {
    const tokens = this.tokens.load();
    if (!tokens) return false;
    try {
      const r = await this.refresh(tokens.refreshToken);
      this.tokens.save({
        deviceId: r.device_id ?? tokens.deviceId,
        syncToken: r.sync_token,
        refreshToken: r.refresh_token,
        expiresAt: Date.now() + r.expires_in * 1000,
      });
      this.onTokensRefreshed?.();
      return true;
    } catch {
      this.tokens.clear();
      return false;
    }
  }
}

export interface PushItem {
  key: string;
  value?: string; // base64
  payload_ver?: number;
  base_version?: number;
  deleted?: boolean;
}

export interface RefreshResponse {
  sync_token: string;
  refresh_token: string;
  expires_in: number;
  device_id?: string;
}

async function toError(res: Response): Promise<BassRestError> {
  let code = 'http_error';
  let message = `HTTP ${res.status}`;
  try {
    const body = (await res.json()) as { error?: string; message?: string };
    if (body.error) code = body.error;
    if (body.message) message = body.message;
  } catch {
    // body wasn't JSON
  }
  return new BassRestError(res.status, code, message);
}
