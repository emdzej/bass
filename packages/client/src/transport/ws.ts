import type { TokenStore } from '../storage/tokens.js';

export interface WSEvents {
  onChange: (cursor: number) => void;
  onOpen?: () => void;
  onClose?: () => void;
  onError?: (err: unknown) => void;
}

const MIN_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 60_000;

/**
 * WSChannel manages the /v1/changes WebSocket: reconnects with exponential
 * backoff, sends `subscribe`, dispatches `change` messages to onChange.
 *
 * Auth rides on Sec-WebSocket-Protocol — browsers accept the second arg of
 * `new WebSocket(url, protocols)` as a string[] and forward it verbatim.
 */
export class WSChannel {
  private ws: WebSocket | null = null;
  private closed = false;
  private backoff = MIN_BACKOFF_MS;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private cursorAccessor: () => number;

  constructor(
    private wsUrl: string,
    private tokens: TokenStore,
    private events: WSEvents,
    cursorAccessor: () => number,
    private WSImpl: typeof WebSocket = WebSocket,
  ) {
    this.cursorAccessor = cursorAccessor;
  }

  start(): void {
    this.closed = false;
    this.connect();
  }

  stop(): void {
    this.closed = true;
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    if (this.ws) {
      try {
        this.ws.close();
      } catch {
        // ignore
      }
      this.ws = null;
    }
  }

  private connect(): void {
    const tokens = this.tokens.load();
    if (!tokens) {
      // not paired; nothing to do
      return;
    }
    let ws: WebSocket;
    try {
      ws = new this.WSImpl(this.wsUrl, ['bass.v1', `bearer.${tokens.syncToken}`]);
    } catch (err) {
      this.events.onError?.(err);
      this.scheduleReconnect();
      return;
    }
    this.ws = ws;

    ws.addEventListener('open', () => {
      this.backoff = MIN_BACKOFF_MS;
      try {
        ws.send(JSON.stringify({ type: 'subscribe', since: this.cursorAccessor() }));
      } catch (err) {
        this.events.onError?.(err);
      }
      this.events.onOpen?.();
    });

    ws.addEventListener('message', (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(String(ev.data));
        if (msg && msg.type === 'change' && typeof msg.cursor === 'number') {
          this.events.onChange(msg.cursor);
        } else if (msg && msg.type === 'ping') {
          ws.send(JSON.stringify({ type: 'pong', t: msg.t }));
        }
      } catch {
        // ignore malformed
      }
    });

    ws.addEventListener('close', () => {
      this.events.onClose?.();
      this.ws = null;
      if (!this.closed) this.scheduleReconnect();
    });

    ws.addEventListener('error', (err: Event) => {
      this.events.onError?.(err);
    });
  }

  private scheduleReconnect(): void {
    if (this.closed) return;
    const jitter = Math.random() * 0.3 * this.backoff;
    this.retryTimer = setTimeout(() => this.connect(), this.backoff + jitter);
    this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF_MS);
  }
}
