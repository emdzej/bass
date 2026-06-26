export type PairMode = 'redirect' | 'popup';

export interface BassClientOptions {
  /** Base URL of the bass service, e.g. https://bass.example. */
  serviceUrl: string;
  /** Registered app id (as configured by the bass admin). */
  appId: string;
  /**
   * Key patterns this client will sync. Strings are matched as case-sensitive
   * globs (`*` matches any character sequence). RegExps are matched directly.
   * Default: ['*'] — sync everything the server allows for this app.
   */
  keys?: (string | RegExp)[];
  /** Outbox flush debounce in ms. Default 500. */
  debounceMs?: number;
  /** Storage backing the proxy + token cache. Default window.localStorage. */
  storage?: Storage;
  /** Override fetch (tests / SSR). Default global fetch. */
  fetchImpl?: typeof fetch;
  /** Override WebSocket constructor (tests). Default global WebSocket. */
  WebSocketImpl?: typeof WebSocket;
}

export interface PairOptions {
  /** Where the IdP returns the user after consent. Must be registered on the app. */
  redirectUri: string;
  /** Transport mode. Default 'redirect'. */
  mode?: PairMode;
  /** Human-readable device label, surfaced in /v1/devices. */
  deviceLabel?: string;
}

export interface DiscoveryConfig {
  issuer?: string;
  scopes: { user: string; admin: string };
  endpoints: {
    pair_start: string;
    pair_callback: string;
    sync: string;
    changes_ws: string;
    token_refresh: string;
    devices: string;
  };
  limits: { max_value_bytes: number; max_batch_items: number };
}

export interface SyncItem {
  key: string;
  value: string; // base64
  payload_ver: number;
  version: number;
  deleted: boolean;
  updated_at: number;
  updated_by: string;
}

export interface PullResponse {
  items: SyncItem[];
  cursor: number;
  has_more: boolean;
}

export interface PushResult {
  key: string;
  status: 'accepted' | 'accepted_overwrite' | 'rejected';
  version?: number;
  previous_version?: number;
}

export interface PushResponse {
  results: PushResult[];
  cursor: number;
}

export interface AuthState {
  isPaired: boolean;
  deviceId?: string;
  expiresAt?: number;
  lastSync?: number;
}

export interface BassDevice {
  id: string;
  user_sub: string;
  app_id: string;
  label?: string;
  token_expires: string;
  created_at: string;
  last_seen_at: string;
  revoked: boolean;
}

export interface HydrateOptions {
  /** Max time to await the initial pull. 0 = non-blocking. Default 2000. */
  timeoutMs?: number;
}

export interface HydrateResult {
  timedOut: boolean;
  itemsApplied: number;
}
